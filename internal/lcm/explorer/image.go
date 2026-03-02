package explorer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ImageExplorer explores image files with pure Go parsing for common formats
// and optional external tool fallback (identify, exiftool).
type ImageExplorer struct {
	formatterProfile OutputProfile
}

// imageExtensions maps recognized image extensions to true. SVG is explicitly
// excluded; it is handled by XMLExplorer.
var imageExtensions = map[string]bool{
	"png":  true,
	"jpg":  true,
	"jpeg": true,
	"gif":  true,
	"bmp":  true,
	"ico":  true,
	"webp": true,
	"tiff": true,
	"tif":  true,
	"raw":  true,
	"cr2":  true,
	"nef":  true,
	"arw":  true,
	"dng":  true,
	"psd":  true,
	"heic": true,
	"heif": true,
	"avif": true,
}

// imageMagic defines magic byte signatures for image formats.
type imageMagic struct {
	signature []byte
	format    string
}

var imageMagicSignatures = []imageMagic{
	{signature: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, format: "PNG"},
	{signature: []byte{0xFF, 0xD8, 0xFF}, format: "JPEG"},
	{signature: []byte("GIF87a"), format: "GIF"},
	{signature: []byte("GIF89a"), format: "GIF"},
	{signature: []byte("BM"), format: "BMP"},
	{signature: []byte("RIFF"), format: "WebP"},                 // WebP is RIFF container; verified below.
	{signature: []byte{0x49, 0x49, 0x2A, 0x00}, format: "TIFF"}, // Little-endian TIFF.
	{signature: []byte{0x4D, 0x4D, 0x00, 0x2A}, format: "TIFF"}, // Big-endian TIFF.
}

func (e *ImageExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if imageExtensions[ext] {
		return true
	}
	// Magic byte fallback.
	return detectImageFormat(content) != ""
}

func (e *ImageExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	format := detectImageFormat(input.Content)
	if format == "" {
		format = strings.ToUpper(ext)
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Image file: %s\n", name)
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Attempt pure Go dimension parsing for supported formats.
	info := parseImageInfo(input.Content, format)
	if info.width > 0 && info.height > 0 {
		fmt.Fprintf(&summary, "Dimensions: %dx%d\n", info.width, info.height)
	} else {
		// Fallback to identify for dimensions.
		dims := identifyDimensions(ctx, input.Content)
		if dims != "" {
			fmt.Fprintf(&summary, "Dimensions: %s\n", dims)
		} else {
			summary.WriteString("Dimensions: unknown\n")
		}
	}

	if info.bitDepth > 0 {
		fmt.Fprintf(&summary, "Bit depth: %d\n", info.bitDepth)
	}
	if info.colorType != "" {
		fmt.Fprintf(&summary, "Color type: %s\n", info.colorType)
	}
	if info.animated {
		summary.WriteString("Animated: yes\n")
	}

	// Enhancement mode: attempt exiftool for richer metadata.
	if e.formatterProfile == OutputProfileEnhancement {
		exif := exiftoolMetadata(ctx, input.Content)
		if exif != "" {
			summary.WriteString("\nEXIF metadata:\n")
			summary.WriteString(exif)
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "image",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// imageInfo holds parsed image metadata.
type imageInfo struct {
	width     uint32
	height    uint32
	bitDepth  int
	colorType string
	animated  bool
}

// parseImageInfo attempts pure Go parsing of common image formats.
func parseImageInfo(content []byte, format string) imageInfo {
	switch format {
	case "PNG":
		return parsePNG(content)
	case "JPEG":
		return parseJPEG(content)
	case "GIF":
		return parseGIF(content)
	case "BMP":
		return parseBMP(content)
	default:
		return imageInfo{}
	}
}

// parsePNG extracts dimensions, bit depth, color type, and APNG detection
// from PNG IHDR chunk at offset 16.
func parsePNG(content []byte) imageInfo {
	// PNG signature (8 bytes) + IHDR length (4 bytes) + "IHDR" (4 bytes) = 16.
	// IHDR data: width (4 BE) + height (4 BE) + bit depth (1) + color type (1).
	// Minimum required: 16 + 4 + 4 + 1 + 1 = 26 bytes; we need 33 for safety.
	if len(content) < 33 {
		return imageInfo{}
	}

	// Verify PNG signature.
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.Equal(content[:8], pngSig) {
		return imageInfo{}
	}

	// Verify IHDR chunk type at offset 12.
	if string(content[12:16]) != "IHDR" {
		return imageInfo{}
	}

	width := binary.BigEndian.Uint32(content[16:20])
	height := binary.BigEndian.Uint32(content[20:24])
	bitDepth := int(content[24])
	colorTypeByte := content[25]

	colorType := pngColorType(colorTypeByte)

	// Scan for acTL chunk (APNG animation control).
	animated := scanPNGChunk(content, "acTL")

	return imageInfo{
		width:     width,
		height:    height,
		bitDepth:  bitDepth,
		colorType: colorType,
		animated:  animated,
	}
}

// pngColorType returns a human-readable string for the PNG color type byte.
func pngColorType(ct byte) string {
	switch ct {
	case 0:
		return "grayscale"
	case 2:
		return "RGB"
	case 3:
		return "indexed (palette)"
	case 4:
		return "grayscale+alpha"
	case 6:
		return "RGBA"
	default:
		return fmt.Sprintf("unknown (%d)", ct)
	}
}

// scanPNGChunk scans for a named chunk in PNG data. It walks the chunk
// chain starting after the 8-byte signature.
func scanPNGChunk(content []byte, name string) bool {
	if len(content) < 12 {
		return false
	}
	offset := 8 // Skip PNG signature.
	for offset+8 <= len(content) {
		if offset+4 > len(content) {
			break
		}
		chunkLen := binary.BigEndian.Uint32(content[offset : offset+4])
		if offset+8 > len(content) {
			break
		}
		chunkType := string(content[offset+4 : offset+8])
		if chunkType == name {
			return true
		}
		// Skip chunk: length(4) + type(4) + data(chunkLen) + CRC(4).
		next := offset + 12 + int(chunkLen)
		if next < offset { // Overflow guard.
			break
		}
		offset = next
	}
	return false
}

// parseJPEG extracts dimensions from SOF markers in the first 64 KB.
func parseJPEG(content []byte) imageInfo {
	if len(content) < 2 {
		return imageInfo{}
	}
	// Verify JPEG SOI marker.
	if content[0] != 0xFF || content[1] != 0xD8 {
		return imageInfo{}
	}

	// Scan for SOF markers (0xFFC0-0xFFC3) in the first 64 KB.
	limit := len(content)
	if limit > 65536 {
		limit = 65536
	}

	offset := 2
	for offset+2 <= limit {
		// Find next marker.
		if content[offset] != 0xFF {
			offset++
			continue
		}
		marker := content[offset+1]
		// Skip fill bytes (0xFF).
		if marker == 0xFF {
			offset++
			continue
		}
		// SOF0-SOF3 markers: 0xC0-0xC3.
		if marker >= 0xC0 && marker <= 0xC3 {
			// SOF segment: length(2) + precision(1) + height(2) + width(2).
			if offset+9 > limit {
				break
			}
			height := binary.BigEndian.Uint16(content[offset+5 : offset+7])
			width := binary.BigEndian.Uint16(content[offset+7 : offset+9])
			return imageInfo{
				width:  uint32(width),
				height: uint32(height),
			}
		}
		// Skip to next marker using segment length.
		if offset+4 > limit {
			break
		}
		segLen := int(binary.BigEndian.Uint16(content[offset+2 : offset+4]))
		if segLen < 2 {
			break
		}
		offset += 2 + segLen
	}
	return imageInfo{}
}

// parseGIF extracts logical screen dimensions from the GIF header.
func parseGIF(content []byte) imageInfo {
	// GIF header: signature (6 bytes) + logical screen descriptor.
	// Logical screen descriptor: width (2 LE) + height (2 LE) at offset 6.
	if len(content) < 10 {
		return imageInfo{}
	}
	sig := string(content[:6])
	if sig != "GIF87a" && sig != "GIF89a" {
		return imageInfo{}
	}
	width := binary.LittleEndian.Uint16(content[6:8])
	height := binary.LittleEndian.Uint16(content[8:10])
	return imageInfo{
		width:  uint32(width),
		height: uint32(height),
	}
}

// parseBMP extracts dimensions and bit depth from the BMP info header.
func parseBMP(content []byte) imageInfo {
	// BMP file header: "BM" (2) + file size (4) + reserved (4) + offset (4) = 14.
	// BMP info header at offset 14: header size (4) + width (4 LE signed) +
	// height (4 LE signed) + planes (2) + bit depth (2).
	if len(content) < 30 {
		return imageInfo{}
	}
	if content[0] != 'B' || content[1] != 'M' {
		return imageInfo{}
	}
	// Width and height are signed int32 LE.
	w := int32(binary.LittleEndian.Uint32(content[18:22]))
	h := int32(binary.LittleEndian.Uint32(content[22:26]))
	// Height can be negative (top-down DIB).
	if h < 0 {
		h = -h
	}
	if w < 0 {
		w = -w
	}
	bitDepth := int(binary.LittleEndian.Uint16(content[28:30]))
	return imageInfo{
		width:    uint32(w),
		height:   uint32(h),
		bitDepth: bitDepth,
	}
}

// detectImageFormat returns the format name from magic bytes, or "" if
// unknown.
func detectImageFormat(content []byte) string {
	for _, m := range imageMagicSignatures {
		if len(content) >= len(m.signature) && bytes.HasPrefix(content, m.signature) {
			// WebP needs additional verification: bytes 8-12 must be "WEBP".
			if m.format == "WebP" {
				if len(content) >= 12 && string(content[8:12]) == "WEBP" {
					return "WebP"
				}
				continue
			}
			return m.format
		}
	}
	return ""
}

// identifyDimensions calls ImageMagick identify to get dimensions for
// formats we cannot parse in pure Go. Returns "" on failure.
func identifyDimensions(ctx context.Context, content []byte) string {
	identifyPath, err := exec.LookPath("identify")
	if err != nil {
		return ""
	}

	var dims string
	err = withTempFile("crush-img-*", content, func(path string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, identifyPath, "-format", "%wx%h", path)
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		dims = strings.TrimSpace(string(out))
		return nil
	})
	if err != nil {
		return ""
	}
	return dims
}

// exiftoolMetadata calls exiftool to extract EXIF metadata. Returns "" on
// failure (optional, non-fatal).
func exiftoolMetadata(ctx context.Context, content []byte) string {
	exiftoolPath, err := exec.LookPath("exiftool")
	if err != nil {
		return ""
	}

	var result string
	err = withTempFile("crush-exif-*", content, func(path string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exiftoolPath, "-s", "-G", path)
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		result = formatExifOutput(string(out))
		return nil
	})
	if err != nil {
		return ""
	}
	return result
}

// formatExifOutput trims and limits exiftool output to relevant fields.
func formatExifOutput(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var filtered []string

	// Interesting EXIF groups/tags.
	interestingPrefixes := []string{
		"[EXIF]",
		"[ICC_Profile]",
		"[Composite]",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, prefix := range interestingPrefixes {
			if strings.HasPrefix(line, prefix) {
				filtered = append(filtered, "  "+line)
				break
			}
		}
	}

	if len(filtered) == 0 {
		return ""
	}

	// Limit to 20 lines.
	if len(filtered) > 20 {
		filtered = filtered[:20]
		filtered = append(filtered, "  ... and more EXIF data")
	}

	return strings.Join(filtered, "\n") + "\n"
}
