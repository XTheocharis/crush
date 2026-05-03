package explorer

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildPNG builds a minimal valid PNG with the given dimensions, bit depth,
// and color type. The result is at least 33 bytes (signature + IHDR).
func buildPNG(width, height uint32, bitDepth byte, colorType byte) []byte {
	// PNG signature (8 bytes).
	sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// IHDR chunk: length (4) + "IHDR" (4) + data (13) + CRC (4) = 25 bytes.
	ihdrData := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdrData[0:4], width)
	binary.BigEndian.PutUint32(ihdrData[4:8], height)
	ihdrData[8] = bitDepth
	ihdrData[9] = colorType
	ihdrData[10] = 0 // Compression method.
	ihdrData[11] = 0 // Filter method.
	ihdrData[12] = 0 // Interlace method.

	var buf []byte
	buf = append(buf, sig...)
	// Chunk length (4 bytes BE).
	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, 13)
	buf = append(buf, lenBytes...)
	buf = append(buf, []byte("IHDR")...)
	buf = append(buf, ihdrData...)
	// CRC placeholder (4 bytes).
	buf = append(buf, 0, 0, 0, 0)
	return buf
}

// buildAPNG builds a minimal PNG with an acTL (animation control) chunk.
func buildAPNG(width, height uint32) []byte {
	base := buildPNG(width, height, 8, 6) // RGBA.
	// Append acTL chunk (8 bytes data: numFrames + numPlays).
	acTLData := make([]byte, 8)
	binary.BigEndian.PutUint32(acTLData[0:4], 2) // 2 frames.
	binary.BigEndian.PutUint32(acTLData[4:8], 0) // Infinite loop.

	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, 8)
	base = append(base, lenBytes...)
	base = append(base, []byte("acTL")...)
	base = append(base, acTLData...)
	base = append(base, 0, 0, 0, 0) // CRC placeholder.
	return base
}

// buildJPEGSOF0 builds a minimal JPEG with SOF0 marker encoding dimensions.
func buildJPEGSOF0(width, height uint16) []byte {
	var buf []byte
	// SOI marker.
	buf = append(buf, 0xFF, 0xD8)
	// APP0 (JFIF) marker with minimal data to be realistic.
	buf = append(buf, 0xFF, 0xE0)
	buf = append(buf, 0x00, 0x10) // Length = 16.
	buf = append(buf, []byte("JFIF\x00")...)
	buf = append(buf, 0x01, 0x01) // Version.
	buf = append(buf, 0x00)       // Units.
	buf = append(buf, 0x00, 0x01) // X density.
	buf = append(buf, 0x00, 0x01) // Y density.
	buf = append(buf, 0x00, 0x00) // Thumbnail.
	// SOF0 marker.
	buf = append(buf, 0xFF, 0xC0)
	buf = append(buf, 0x00, 0x11)                     // Length = 17 (8 + 3*3).
	buf = append(buf, 0x08)                           // Precision (8 bits).
	hBytes := []byte{byte(height >> 8), byte(height)} // Height BE.
	wBytes := []byte{byte(width >> 8), byte(width)}   // Width BE.
	buf = append(buf, hBytes...)
	buf = append(buf, wBytes...)
	buf = append(buf, 0x03)       // Number of components.
	buf = append(buf, 1, 0x22, 0) // Y component.
	buf = append(buf, 2, 0x11, 1) // Cb component.
	buf = append(buf, 3, 0x11, 1) // Cr component.
	return buf
}

// buildGIF builds a minimal GIF89a header with the given dimensions.
func buildGIF(width, height uint16) []byte {
	var buf []byte
	buf = append(buf, []byte("GIF89a")...)
	wBytes := make([]byte, 2)
	hBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(wBytes, width)
	binary.LittleEndian.PutUint16(hBytes, height)
	buf = append(buf, wBytes...)
	buf = append(buf, hBytes...)
	return buf
}

// buildBMP builds a minimal BMP header with the given dimensions and bit
// depth.
func buildBMP(width, height int32, bitDepth uint16) []byte {
	buf := make([]byte, 30)
	buf[0] = 'B'
	buf[1] = 'M'
	// File size at offset 2 (not critical for parsing).
	binary.LittleEndian.PutUint32(buf[2:6], 30)
	// Reserved (4 bytes) at offset 6.
	// Data offset at offset 10.
	binary.LittleEndian.PutUint32(buf[10:14], 26)
	// Info header size at offset 14.
	binary.LittleEndian.PutUint32(buf[14:18], 12)
	// Width at offset 18.
	binary.LittleEndian.PutUint32(buf[18:22], uint32(width))
	// Height at offset 22.
	binary.LittleEndian.PutUint32(buf[22:26], uint32(height))
	// Planes at offset 26.
	binary.LittleEndian.PutUint16(buf[26:28], 1)
	// Bit depth at offset 28.
	binary.LittleEndian.PutUint16(buf[28:30], bitDepth)
	return buf
}

func TestImageExplorer_CanHandle(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		// Extension-based detection (18 extensions).
		{name: "png extension", path: "photo.png", content: nil, expected: true},
		{name: "jpg extension", path: "photo.jpg", content: nil, expected: true},
		{name: "jpeg extension", path: "photo.jpeg", content: nil, expected: true},
		{name: "gif extension", path: "anim.gif", content: nil, expected: true},
		{name: "bmp extension", path: "image.bmp", content: nil, expected: true},
		{name: "ico extension", path: "favicon.ico", content: nil, expected: true},
		{name: "webp extension", path: "photo.webp", content: nil, expected: true},
		{name: "tiff extension", path: "scan.tiff", content: nil, expected: true},
		{name: "tif extension", path: "scan.tif", content: nil, expected: true},
		{name: "raw extension", path: "photo.raw", content: nil, expected: true},
		{name: "cr2 extension", path: "photo.cr2", content: nil, expected: true},
		{name: "nef extension", path: "photo.nef", content: nil, expected: true},
		{name: "arw extension", path: "photo.arw", content: nil, expected: true},
		{name: "dng extension", path: "photo.dng", content: nil, expected: true},
		{name: "psd extension", path: "design.psd", content: nil, expected: true},
		{name: "heic extension", path: "photo.heic", content: nil, expected: true},
		{name: "heif extension", path: "photo.heif", content: nil, expected: true},
		{name: "avif extension", path: "photo.avif", content: nil, expected: true},
		{name: "uppercase PNG", path: "PHOTO.PNG", content: nil, expected: true},
		// SVG is NOT claimed.
		{name: "svg not claimed", path: "icon.svg", content: nil, expected: false},
		// Non-image extensions.
		{name: "go file", path: "main.go", content: nil, expected: false},
		{name: "txt file", path: "readme.txt", content: nil, expected: false},
		{name: "json file", path: "data.json", content: nil, expected: false},
		// Magic byte fallback.
		{
			name:     "PNG magic bytes no extension",
			path:     "unknown_file",
			content:  buildPNG(100, 100, 8, 6),
			expected: true,
		},
		{
			name:     "JPEG magic bytes no extension",
			path:     "unknown_file",
			content:  buildJPEGSOF0(640, 480),
			expected: true,
		},
		{
			name:     "GIF magic bytes no extension",
			path:     "unknown_file",
			content:  buildGIF(320, 240),
			expected: true,
		},
		{
			name:     "BMP magic bytes no extension",
			path:     "unknown_file",
			content:  buildBMP(800, 600, 24),
			expected: true,
		},
		{
			name:     "WebP magic bytes no extension",
			path:     "unknown_file",
			content:  []byte("RIFF\x00\x00\x00\x00WEBP"),
			expected: true,
		},
		{
			name:     "TIFF LE magic bytes no extension",
			path:     "unknown_file",
			content:  []byte{0x49, 0x49, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: true,
		},
		{
			name:     "TIFF BE magic bytes no extension",
			path:     "unknown_file",
			content:  []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x00},
			expected: true,
		},
		{
			name:     "random bytes no extension",
			path:     "unknown_file",
			content:  []byte{0x00, 0x01, 0x02, 0x03},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := explorer.CanHandle(tt.path, tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestImageExplorer_PNG(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildPNG(1920, 1080, 8, 6) // 1920x1080 RGBA.

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "screenshot.png",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)
	require.Contains(t, result.Summary, "Image file: screenshot.png")
	require.Contains(t, result.Summary, "Format: PNG")
	require.Contains(t, result.Summary, "Dimensions: 1920x1080")
	require.Contains(t, result.Summary, "Bit depth: 8")
	require.Contains(t, result.Summary, "Color type: RGBA")
}

func TestImageExplorer_PNG_Grayscale(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildPNG(256, 256, 16, 0) // 256x256 grayscale 16-bit.

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "gray.png",
		Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Dimensions: 256x256")
	require.Contains(t, result.Summary, "Bit depth: 16")
	require.Contains(t, result.Summary, "Color type: grayscale")
}

func TestImageExplorer_APNG(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildAPNG(320, 240)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "anim.png",
		Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Animated: yes")
	require.Contains(t, result.Summary, "Dimensions: 320x240")
}

func TestImageExplorer_JPEG(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildJPEGSOF0(4032, 3024)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "photo.jpg",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: JPEG")
	require.Contains(t, result.Summary, "Dimensions: 4032x3024")
}

func TestImageExplorer_GIF(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildGIF(640, 480)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "animation.gif",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: GIF")
	require.Contains(t, result.Summary, "Dimensions: 640x480")
}

func TestImageExplorer_BMP(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	content := buildBMP(1024, 768, 24)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "bitmap.bmp",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: BMP")
	require.Contains(t, result.Summary, "Dimensions: 1024x768")
	require.Contains(t, result.Summary, "Bit depth: 24")
}

func TestImageExplorer_BMP_TopDown(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	// Top-down BMP has negative height.
	content := buildBMP(800, -600, 32)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "topdown.bmp",
		Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Dimensions: 800x600")
}

func TestImageExplorer_UnknownFormat(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}
	// HEIC extension but no parseable content.
	content := []byte("not a real image file but has enough bytes to avoid panics")

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "photo.heic",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: HEIC")
	// Without identify installed, dimensions should be "unknown".
	require.Contains(t, result.Summary, "Dimensions:")
}

func TestImageExplorer_MagicBytesTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "PNG signature",
			content:  []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "PNG",
		},
		{
			name:     "JPEG signature",
			content:  []byte{0xFF, 0xD8, 0xFF, 0xE0},
			expected: "JPEG",
		},
		{
			name:     "GIF87a signature",
			content:  []byte("GIF87a"),
			expected: "GIF",
		},
		{
			name:     "GIF89a signature",
			content:  []byte("GIF89a"),
			expected: "GIF",
		},
		{
			name:     "BMP signature",
			content:  []byte("BM\x00\x00"),
			expected: "BMP",
		},
		{
			name:     "WebP signature",
			content:  []byte("RIFF\x00\x00\x00\x00WEBP"),
			expected: "WebP",
		},
		{
			name:     "RIFF without WEBP is not image",
			content:  []byte("RIFF\x00\x00\x00\x00WAVE"),
			expected: "",
		},
		{
			name:     "TIFF little-endian",
			content:  []byte{0x49, 0x49, 0x2A, 0x00},
			expected: "TIFF",
		},
		{
			name:     "TIFF big-endian",
			content:  []byte{0x4D, 0x4D, 0x00, 0x2A},
			expected: "TIFF",
		},
		{
			name:     "unknown bytes",
			content:  []byte{0x00, 0x01, 0x02},
			expected: "",
		},
		{
			name:     "empty content",
			content:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := detectImageFormat(tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestImageExplorer_PureParsing(t *testing.T) {
	t.Parallel()

	t.Run("PNG IHDR parsing", func(t *testing.T) {
		t.Parallel()
		info := parsePNG(buildPNG(3840, 2160, 8, 2))
		require.Equal(t, uint32(3840), info.width)
		require.Equal(t, uint32(2160), info.height)
		require.Equal(t, 8, info.bitDepth)
		require.Equal(t, "RGB", info.colorType)
		require.False(t, info.animated)
	})

	t.Run("PNG too short", func(t *testing.T) {
		t.Parallel()
		info := parsePNG([]byte{0x89, 0x50, 0x4E, 0x47})
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("PNG bad signature", func(t *testing.T) {
		t.Parallel()
		bad := buildPNG(100, 100, 8, 6)
		bad[0] = 0x00 // Corrupt signature.
		info := parsePNG(bad)
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("JPEG SOF0 parsing", func(t *testing.T) {
		t.Parallel()
		info := parseJPEG(buildJPEGSOF0(1920, 1080))
		require.Equal(t, uint32(1920), info.width)
		require.Equal(t, uint32(1080), info.height)
	})

	t.Run("JPEG too short", func(t *testing.T) {
		t.Parallel()
		info := parseJPEG([]byte{0xFF})
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("GIF parsing", func(t *testing.T) {
		t.Parallel()
		info := parseGIF(buildGIF(800, 600))
		require.Equal(t, uint32(800), info.width)
		require.Equal(t, uint32(600), info.height)
	})

	t.Run("GIF too short", func(t *testing.T) {
		t.Parallel()
		info := parseGIF([]byte("GIF89"))
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("GIF bad signature", func(t *testing.T) {
		t.Parallel()
		info := parseGIF([]byte("GIF99a\x00\x00\x00\x00"))
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("BMP parsing", func(t *testing.T) {
		t.Parallel()
		info := parseBMP(buildBMP(2560, 1440, 32))
		require.Equal(t, uint32(2560), info.width)
		require.Equal(t, uint32(1440), info.height)
		require.Equal(t, 32, info.bitDepth)
	})

	t.Run("BMP too short", func(t *testing.T) {
		t.Parallel()
		info := parseBMP([]byte("BM"))
		require.Equal(t, uint32(0), info.width)
	})

	t.Run("BMP negative height", func(t *testing.T) {
		t.Parallel()
		info := parseBMP(buildBMP(100, -200, 24))
		require.Equal(t, uint32(100), info.width)
		require.Equal(t, uint32(200), info.height)
	})
}

func TestImageExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	content := buildPNG(1024, 768, 8, 6)

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path:    "test.png",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "image", result.ExplorerUsed)
	require.Contains(t, result.Summary, "1024x768")
}

func TestImageExplorer_SVGNotClaimed(t *testing.T) {
	t.Parallel()

	explorer := &ImageExplorer{}

	// SVG by extension.
	require.False(t, explorer.CanHandle("icon.svg", nil))
	require.False(t, explorer.CanHandle("ICON.SVG", nil))

	// SVG content (should not match image magic).
	svgContent := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`)
	require.False(t, explorer.CanHandle("icon", svgContent))
}
