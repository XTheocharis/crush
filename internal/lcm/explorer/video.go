package explorer

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// VideoExplorer explores video files (MP4, MKV, AVI, MOV).
// Parses container headers. No external dependencies or CGO.
type VideoExplorer struct{}

var videoExtensions = map[string]bool{
	"mp4":  true,
	"m4v":  true,
	"mkv":  true,
	"webm": true,
	"avi":  true,
	"mov":  true,
	"wmv":  true,
	"flv":  true,
}

func (e *VideoExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if videoExtensions[ext] {
		return true
	}
	return detectVideoFormat(content) != ""
}

func (e *VideoExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	format := detectVideoFormat(input.Content)
	if format == "" {
		format = strings.ToUpper(ext)
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Video file: %s\n", name)
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	info := parseVideoInfo(input.Content, format)

	if info.duration > 0 {
		fmt.Fprintf(&summary, "Duration: %s\n", formatDuration(info.duration))
	}
	if info.width > 0 && info.height > 0 {
		fmt.Fprintf(&summary, "Resolution: %dx%d\n", info.width, info.height)
	}
	if info.codec != "" {
		fmt.Fprintf(&summary, "Codec: %s\n", info.codec)
	}
	if info.bitrate > 0 {
		fmt.Fprintf(&summary, "Bitrate: %d kbps\n", info.bitrate)
	}
	if info.fps > 0 {
		fmt.Fprintf(&summary, "Frame rate: %.1f fps\n", info.fps)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "video",
		TokenEstimate: estimateTokens(result),
	}, nil
}

type videoInfo struct {
	duration float64 // seconds
	width    int
	height   int
	codec    string
	bitrate  int // kbps
	fps      float64
}

func parseVideoInfo(content []byte, format string) videoInfo {
	switch format {
	case "MP4", "MOV":
		return parseMP4(content)
	case "MKV", "WebM":
		return parseMKV(content)
	case "AVI":
		return parseAVI(content)
	default:
		return videoInfo{}
	}
}

// parseMP4 parses ISO Base Media File Format (MP4/MOV) box structure.
func parseMP4(content []byte) videoInfo {
	info := videoInfo{}
	parseMP4Boxes(content, 0, len(content), &info, 0)
	if info.duration > 0 && info.bitrate == 0 {
		info.bitrate = len(content) * 8 / int(info.duration) / 1000
	}
	return info
}

// parseMP4Boxes recursively walks MP4 box structure extracting metadata.
func parseMP4Boxes(content []byte, start, end int, info *videoInfo, depth int) {
	if depth > 10 {
		return
	}

	offset := start
	for offset+8 <= end {
		boxSize := int(binary.BigEndian.Uint32(content[offset : offset+4]))
		boxType := string(content[offset+4 : offset+8])

		if boxSize < 8 {
			break
		}

		boxEnd := offset + boxSize
		if boxEnd > end {
			break
		}

		switch boxType {
		case "moov", "trak", "mdia", "minf", "stbl":
			// Container boxes — recurse.
			parseMP4Boxes(content, offset+8, boxEnd, info, depth+1)

		case "mvhd":
			parseMVHD(content, offset+8, boxEnd, info)

		case "tkhd":
			parseTKHD(content, offset+8, boxEnd, info)

		case "stsd":
			parseSTSD(content, offset+8, boxEnd, info)

		case "mdhd":
			parseMDHD(content, offset+8, boxEnd, info)
		}

		offset = boxEnd
	}
}

// parseMVHD extracts duration from the Movie Header Box.
func parseMVHD(content []byte, start, end int, info *videoInfo) {
	if start+100 > end || start+100 > len(content) {
		return
	}
	version := content[start]
	if version == 0 {
		timeScale := binary.BigEndian.Uint32(content[start+12 : start+16])
		duration := binary.BigEndian.Uint32(content[start+16 : start+20])
		if timeScale > 0 {
			info.duration = float64(duration) / float64(timeScale)
		}
	} else {
		// version 1: 64-bit duration.
		if start+36 > end || start+36 > len(content) {
			return
		}
		timeScale := binary.BigEndian.Uint32(content[start+20 : start+24])
		duration := binary.BigEndian.Uint64(content[start+24 : start+32])
		if timeScale > 0 {
			info.duration = float64(duration) / float64(timeScale)
		}
	}
}

// parseMDHD extracts track duration from Media Header Box.
func parseMDHD(content []byte, start, end int, info *videoInfo) {
	if start+24 > end || start+24 > len(content) {
		return
	}
	version := content[start]
	if version == 0 {
		timeScale := binary.BigEndian.Uint32(content[start+12 : start+16])
		duration := binary.BigEndian.Uint32(content[start+16 : start+20])
		if timeScale > 0 && info.duration == 0 {
			info.duration = float64(duration) / float64(timeScale)
		}
	}
}

// parseTKHD extracts width/height from Track Header Box.
func parseTKHD(content []byte, start, end int, info *videoInfo) {
	if start+84 > end || start+84 > len(content) {
		return
	}
	version := content[start]
	if version == 0 {
		widthFixed := binary.BigEndian.Uint32(content[start+76 : start+80])
		heightFixed := binary.BigEndian.Uint32(content[start+80 : start+84])
		w := float64(widthFixed) / 65536.0
		h := float64(heightFixed) / 65536.0
		if w > 0 && h > 0 {
			info.width = int(math.Round(w))
			info.height = int(math.Round(h))
		}
	}
}

// parseSTSD extracts codec from Sample Description Box.
func parseSTSD(content []byte, start, end int, info *videoInfo) {
	if start+8 > end || start+8 > len(content) {
		return
	}
	entryCount := binary.BigEndian.Uint32(content[start+4 : start+8])
	if entryCount == 0 {
		return
	}
	entryStart := start + 8
	if entryStart+16 > end || entryStart+16 > len(content) {
		return
	}
	// First entry: size(4) + format(4).
	codec := string(content[entryStart+4 : entryStart+8])
	switch codec {
	case "avc1", "avc3":
		info.codec = "H.264"
	case "hev1", "hvc1":
		info.codec = "H.265/HEVC"
	case "mp4v":
		info.codec = "MPEG-4"
	case "vp09":
		info.codec = "VP9"
	case "av01":
		info.codec = "AV1"
	case "mp4a":
		info.codec = "AAC"
	default:
		if isPrintableASCII(codec) {
			info.codec = codec
		}
	}
}

// parseMKV parses Matroska/WebM EBML header for video metadata.
func parseMKV(content []byte) videoInfo {
	info := videoInfo{}
	if len(content) < 16 {
		return info
	}

	// Verify EBML header.
	elementID, _, _ := readEBMLHeader(content, 0)
	if elementID != 0x1A45DFA3 { // EBML.
		return info
	}

	parseEBMLElements(content, 0, min(len(content), 1<<20), &info, 0)

	if info.duration > 0 && info.bitrate == 0 {
		info.bitrate = len(content) * 8 / int(info.duration) / 1000
	}
	return info
}

// parseEBMLElements walks EBML element tree extracting video metadata.
func parseEBMLElements(content []byte, start, end int, info *videoInfo, depth int) {
	if depth > 8 {
		return
	}

	offset := start
	for offset < end {
		elementID, dataSize, headerLen := readEBMLHeader(content, offset)
		if elementID == 0 || headerLen == 0 {
			break
		}

		dataStart := offset + headerLen
		dataEnd := dataStart + dataSize
		if dataEnd > end {
			dataEnd = end
		}
		if dataStart > dataEnd {
			break
		}

		switch elementID {
		case 0x1549A966: // SegmentInfo → recurse.
			parseEBMLElements(content, dataStart, dataEnd, info, depth+1)

		case 0x2AD7B1: // TimecodeScale.
			if dataStart+4 <= dataEnd && dataStart+4 <= len(content) {
				scale := readEBMLUint(content, dataStart, dataSize)
				if scale > 0 {
					_ = scale // nanoseconds per tick.
				}
			}

		case 0x4489: // Duration.
			if dataStart+8 <= dataEnd && dataStart+8 <= len(content) {
				info.duration = readEBMLFloat(content, dataStart)
			}

		case 0x1654AE6B: // Tracks → recurse.
			parseEBMLElements(content, dataStart, dataEnd, info, depth+1)

		case 0xAE: // TrackEntry → recurse.
			parseEBMLElements(content, dataStart, dataEnd, info, depth+1)

		case 0xE0: // Video → recurse.
			parseEBMLElements(content, dataStart, dataEnd, info, depth+1)

		case 0xB0: // PixelWidth.
			if dataStart+4 <= dataEnd && dataStart+4 <= len(content) {
				info.width = int(readEBMLUint(content, dataStart, dataSize))
			}

		case 0xBA: // PixelHeight.
			if dataStart+4 <= dataEnd && dataStart+4 <= len(content) {
				info.height = int(readEBMLUint(content, dataStart, dataSize))
			}

		case 0x83: // CodecID.
			if dataStart+int(dataSize) <= len(content) {
				codecID := string(content[dataStart : dataStart+dataSize])
				info.codec = mkvCodecName(codecID)
			}
		}

		nextOffset := dataEnd
		if nextOffset <= offset {
			break
		}
		offset = nextOffset
	}
}

// readEBMLHeader reads an EBML element ID and data size from content.
func readEBMLHeader(content []byte, offset int) (elementID uint32, dataSize int, headerLen int) {
	if offset >= len(content) {
		return 0, 0, 0
	}

	// Read element ID (variable length 1-4 bytes).
	id, idLen := readEBMLVInt(content, offset)
	if idLen == 0 {
		return 0, 0, 0
	}

	// Read data size (variable length 1-8 bytes).
	size, sizeLen := readEBMLVInt(content, offset+idLen)
	if sizeLen == 0 {
		return 0, 0, 0
	}

	// Unknown/unbounded size.
	if size == 0x01FFFFFFFFFFFFFF {
		size = uint64(len(content) - offset - idLen - sizeLen)
	}

	return uint32(id), int(size), idLen + sizeLen
}

// readEBMLVInt reads a variable-length integer per EBML spec.
func readEBMLVInt(content []byte, offset int) (value uint64, length int) {
	if offset >= len(content) {
		return 0, 0
	}

	first := content[offset]
	var lengthMask byte
	var vintLen int

	switch {
	case first&0x80 != 0:
		vintLen = 1
		lengthMask = 0x7F
	case first&0x40 != 0:
		vintLen = 2
		lengthMask = 0x3F
	case first&0x20 != 0:
		vintLen = 3
		lengthMask = 0x1F
	case first&0x10 != 0:
		vintLen = 4
		lengthMask = 0x0F
	case first&0x08 != 0:
		vintLen = 5
		lengthMask = 0x07
	case first&0x04 != 0:
		vintLen = 6
		lengthMask = 0x03
	case first&0x02 != 0:
		vintLen = 7
		lengthMask = 0x01
	case first&0x01 != 0:
		vintLen = 8
		lengthMask = 0x00
	default:
		return 0, 0
	}

	if offset+vintLen > len(content) {
		return 0, 0
	}

	var val uint64
	for i := 0; i < vintLen; i++ {
		val = (val << 8) | uint64(content[offset+i])
	}
	val &= (1 << (uint(vintLen)*8 - uint(vintLen-1))) - 1
	if vintLen == 1 {
		val = uint64(first & lengthMask)
	} else {
		// Mask out the length marker bits.
		mask := uint64(lengthMask)
		val = val & ((mask << (uint(vintLen-1) * 8)) | 0xFF<<(uint(vintLen-2)*8) | 0xFF)
	}

	return val, vintLen
}

// readEBMLUint reads an EBML unsigned integer value.
func readEBMLUint(content []byte, offset int, size int) uint64 {
	if offset+size > len(content) || size > 8 {
		return 0
	}
	var val uint64
	for i := 0; i < size; i++ {
		val = (val << 8) | uint64(content[offset+i])
	}
	return val
}

// readEBMLFloat reads an EBML float value (4 or 8 bytes).
func readEBMLFloat(content []byte, offset int) float64 {
	if offset+8 <= len(content) {
		bits := binary.BigEndian.Uint64(content[offset : offset+8])
		return math.Float64frombits(bits)
	}
	if offset+4 <= len(content) {
		bits := binary.BigEndian.Uint32(content[offset : offset+4])
		return float64(math.Float32frombits(bits))
	}
	return 0
}

// mkvCodecName maps Matroska CodecID strings to readable names.
func mkvCodecName(codecID string) string {
	switch {
	case strings.HasPrefix(codecID, "V_MPEG4/ISO/AVC"):
		return "H.264"
	case strings.HasPrefix(codecID, "V_MPEGH/ISO/HEVC"):
		return "H.265/HEVC"
	case strings.HasPrefix(codecID, "V_MPEG4/ISO/ASP"):
		return "MPEG-4"
	case strings.HasPrefix(codecID, "V_VP8"):
		return "VP8"
	case strings.HasPrefix(codecID, "V_VP9"):
		return "VP9"
	case strings.HasPrefix(codecID, "V_AV1"):
		return "AV1"
	case strings.HasPrefix(codecID, "A_AAC"):
		return "AAC"
	case strings.HasPrefix(codecID, "A_VORBIS"):
		return "Vorbis"
	case strings.HasPrefix(codecID, "A_OPUS"):
		return "Opus"
	default:
		return codecID
	}
}

// parseAVI parses AVI RIFF structure for video metadata.
func parseAVI(content []byte) videoInfo {
	info := videoInfo{}
	if len(content) < 12 {
		return info
	}
	if string(content[:4]) != "RIFF" || string(content[8:12]) != "AVI " {
		return info
	}

	parseAVIChunks(content, 12, len(content), &info)

	if info.duration > 0 && info.bitrate == 0 {
		info.bitrate = len(content) * 8 / int(info.duration) / 1000
	}
	return info
}

// parseAVIChunks walks AVI RIFF chunks extracting video metadata.
func parseAVIChunks(content []byte, start, end int, info *videoInfo) {
	offset := start
	for offset+8 <= end {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > end {
			dataEnd = end
		}

		switch chunkID {
		case "LIST":
			if dataStart+4 <= dataEnd {
				listType := string(content[dataStart : dataStart+4])
				if listType == "hdrl" || listType == "strl" {
					parseAVIChunks(content, dataStart+4, dataEnd, info)
				}
			}
		case "avih":
			if dataStart+56 <= dataEnd && dataStart+56 <= len(content) {
				microsecPerFrame := binary.LittleEndian.Uint32(content[dataStart : dataStart+4])
				totalFrames := binary.LittleEndian.Uint32(content[dataStart+16 : dataStart+20])
				if microsecPerFrame > 0 {
					info.duration = float64(totalFrames) * float64(microsecPerFrame) / 1_000_000.0
					info.fps = 1_000_000.0 / float64(microsecPerFrame)
				}
			}
		case "strh":
			if dataStart+56 <= dataEnd && dataStart+56 <= len(content) {
				fccType := string(content[dataStart : dataStart+4])
				if fccType == "vids" {
					fccHandler := string(content[dataStart+4 : dataStart+8])
					if isPrintableASCII(fccHandler) {
						info.codec = aviCodecName(fccHandler)
					}
				}
			}
		case "strf":
			if dataStart+40 <= dataEnd && dataStart+40 <= len(content) {
				biWidth := binary.LittleEndian.Uint32(content[dataStart+4 : dataStart+8])
				biHeight := binary.LittleEndian.Uint32(content[dataStart+8 : dataStart+12])
				if biWidth > 0 && biHeight > 0 {
					info.width = int(biWidth)
					info.height = int(biHeight)
				}
			}
		}

		offset = dataEnd
		if chunkSize == 0 {
			break
		}
		// Align to 2-byte boundary.
		if offset%2 != 0 {
			offset++
		}
	}
}

// aviCodecName maps AVI FourCC codes to readable names.
func aviCodecName(fourcc string) string {
	switch fourcc {
	case "H264", "X264", "AVC1":
		return "H.264"
	case "HEVC", "H265", "X265":
		return "H.265/HEVC"
	case "DIVX", "DX50", "XVID", "FMP4":
		return "MPEG-4"
	case "WMV3":
		return "WMV9"
	default:
		return fourcc
	}
}

// detectVideoFormat returns the format name from magic bytes.
func detectVideoFormat(content []byte) string {
	if len(content) < 12 {
		return ""
	}
	switch {
	case len(content) >= 8 && (string(content[4:8]) == "ftyp"):
		return "MP4"
	case string(content[:4]) == "RIFF" && string(content[8:12]) == "AVI ":
		return "AVI"
	case len(content) >= 4 && string(content[:4]) == "\x1A\x45\xDF\xA3":
		return "MKV"
	case string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP":
		return ""
	default:
		return ""
	}
}

// isPrintableASCII checks if all bytes in s are printable ASCII.
func isPrintableASCII(s string) bool {
	for _, b := range []byte(s) {
		if b < 0x20 || b > 0x7E {
			return false
		}
	}
	return len(s) > 0
}
