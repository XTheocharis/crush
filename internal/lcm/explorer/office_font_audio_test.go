package explorer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficeExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"docx", "document.docx", true},
		{"xlsx", "spreadsheet.xlsx", true},
		{"pptx", "presentation.pptx", true},
		{"odt", "doc.odt", true},
		{"ods", "sheet.ods", true},
		{"odp", "slides.odp", true},
		{"doc legacy", "old.doc", true},
		{"xls legacy", "old.xls", true},
		{"ppt legacy", "old.ppt", true},
		{"uppercase", "test.DOCX", true},
		{"go file", "main.go", false},
		{"txt file", "readme.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, explorer.CanHandle(tt.path, nil))
		})
	}
}

func TestOfficeExplorer_CanHandle_ZIPWithContentTypes(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}
	content := buildMinimalOOXML(nil)
	require.True(t, explorer.CanHandle("unknown_file", content))
	require.False(t, explorer.CanHandle("unknown_file", buildPlainZIP()))
}

func TestOfficeExplorer_DOCX(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	coreProps := `<?xml version="1.0"?><cp:coreProperties xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"><dc:title>Test Document</dc:title><dc:creator>Jane Doe</dc:creator></cp:coreProperties>`
	appProps := `<?xml version="1.0"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Pages>5</Pages><Words>1000</Words></Properties>`
	docXML := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello</w:t></w:r></w:p><w:p><w:r><w:t>World</w:t></w:r></w:p></w:body></w:document>`

	content := buildMinimalOOXML(map[string]string{
		"docProps/core.xml": coreProps,
		"docProps/app.xml":  appProps,
		"word/document.xml": docXML,
	})

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.docx", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "office", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Office document: test.docx")
	require.Contains(t, result.Summary, "Word processing (DOCX)")
	require.Contains(t, result.Summary, "Title: Test Document")
	require.Contains(t, result.Summary, "Author: Jane Doe")
	require.Contains(t, result.Summary, "Pages: 5")
	require.Contains(t, result.Summary, "Words: 1000")
	require.Contains(t, result.Summary, "Paragraphs: 2")
}

func TestOfficeExplorer_XLSX(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	workbookXML := `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/><sheet name="Data" sheetId="2" r:id="rId2"/></sheets></workbook>`

	content := buildMinimalOOXML(map[string]string{
		"xl/workbook.xml": workbookXML,
	})

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "report.xlsx", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Spreadsheet (XLSX)")
	require.Contains(t, result.Summary, "Sheets (2)")
	require.Contains(t, result.Summary, "1. Sheet1")
	require.Contains(t, result.Summary, "2. Data")
}

func TestOfficeExplorer_PPTX(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	content := buildMinimalOOXMLWithSlides(3)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "deck.pptx", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Presentation (PPTX)")
	require.Contains(t, result.Summary, "Slides: 3")
}

func TestOfficeExplorer_LegacyFormat(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "old.doc",
		Content: []byte{0xD0, 0xCF, 0x11, 0xE0},
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Legacy binary Office format")
}

func TestOfficeExplorer_CorruptFile(t *testing.T) {
	t.Parallel()
	explorer := &OfficeExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "broken.docx",
		Content: []byte("not a real zip file"),
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Error: unable to parse as ZIP")
}

func TestOfficeExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	content := buildMinimalOOXML(nil)

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path: "test.docx", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "office", result.ExplorerUsed)
}

// buildMinimalOOXML creates a minimal OOXML ZIP file with optional extra files.
func buildMinimalOOXML(extraFiles map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/></Types>`
	f, _ := w.Create("[Content_Types].xml")
	f.Write([]byte(contentTypes))

	for name, data := range extraFiles {
		f, _ := w.Create(name)
		f.Write([]byte(data))
	}
	w.Close()
	return buf.Bytes()
}

// buildMinimalOOXMLWithSlides creates a minimal PPTX with the given number of slides.
func buildMinimalOOXMLWithSlides(numSlides int) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/></Types>`
	f, _ := w.Create("[Content_Types].xml")
	f.Write([]byte(contentTypes))

	for i := 1; i <= numSlides; i++ {
		name := ""
		if i < 10 {
			name = "ppt/slides/slide0" + intToStr(i) + ".xml"
		} else {
			name = "ppt/slides/slide" + intToStr(i) + ".xml"
		}
		f, _ = w.Create(name)
		f.Write([]byte(`<xml/>`))
	}
	w.Close()
	return buf.Bytes()
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// buildPlainZIP creates a minimal ZIP without Office content types.
func buildPlainZIP() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("hello.txt")
	f.Write([]byte("hello world"))
	w.Close()
	return buf.Bytes()
}

func TestFontExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"ttf", "font.ttf", true},
		{"otf", "font.otf", true},
		{"woff", "font.woff", true},
		{"woff2", "font.woff2", true},
		{"ttc", "fonts.ttc", true},
		{"eot", "font.eot", true},
		{"uppercase", "FONT.TTF", true},
		{"go file", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, explorer.CanHandle(tt.path, nil))
		})
	}
}

func TestFontExplorer_CanHandle_MagicBytes(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}

	// TrueType magic: 00 01 00 00.
	ttfMagic := []byte{0x00, 0x01, 0x00, 0x00}
	require.True(t, explorer.CanHandle("unknown", ttfMagic))

	// OpenType magic: "OTTO".
	otfMagic := []byte("OTTO")
	require.True(t, explorer.CanHandle("unknown", otfMagic))

	// WOFF magic: "wOFF".
	woffMagic := []byte("wOFF")
	require.True(t, explorer.CanHandle("unknown", woffMagic))

	// Random bytes.
	require.False(t, explorer.CanHandle("unknown", []byte{0x00, 0x01, 0x02, 0x03}))
}

func TestFontExplorer_TrueType(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}
	content := buildMinimalTTF("TestFont", "Regular", "TestFont Regular", 400, false, 100)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.ttf", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "font", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Font file: test.ttf")
	require.Contains(t, result.Summary, "Format: TrueType")
	require.Contains(t, result.Summary, "Family: TestFont")
	require.Contains(t, result.Summary, "Style: Regular")
	require.Contains(t, result.Summary, "Weight: 400")
	require.Contains(t, result.Summary, "Glyphs: 100")
}

func TestFontExplorer_Italic(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}
	content := buildMinimalTTF("MyFont", "Italic", "MyFont Italic", 400, true, 200)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "italic.ttf", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Italic: yes")
	require.Contains(t, result.Summary, "Weight: 400 (Regular)")
}

func TestFontExplorer_Bold(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}
	content := buildMinimalTTF("MyFont", "Bold", "MyFont Bold", 700, false, 150)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "bold.ttf", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Weight: 700 (Bold)")
}

func TestFontExplorer_ShortContent(t *testing.T) {
	t.Parallel()
	explorer := &FontExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "short.ttf", Content: []byte{0x00, 0x01},
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Font file: short.ttf")
}

func TestFontExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	content := buildMinimalTTF("RegistryFont", "Regular", "RegistryFont", 400, false, 50)

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path: "test.ttf", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "font", result.ExplorerUsed)
}

// buildMinimalTTF creates a minimal TrueType font with given properties.
func buildMinimalTTF(family, subFamily, fullName string, weight uint16, italic bool, numGlyphs uint16) []byte {
	// Build a minimal SFNT with head, name, maxp, and OS/2 tables.
	var tables [][]byte

	// head table (54 bytes).
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm.
	tables = append(tables, head)

	// name table.
	nameData := buildNameTable(family, subFamily, fullName)
	tables = append(tables, nameData)

	// maxp table (6 bytes).
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	tables = append(tables, maxp)

	// OS/2 table (78 bytes minimum).
	os2 := make([]byte, 78)
	binary.BigEndian.PutUint16(os2[4:6], weight)
	if italic {
		os2[63] = 0x01 // fsSelection bit 0 = italic (low byte of BE uint16).
	}
	tables = append(tables, os2)

	// Build SFNT container.
	numTables := uint16(len(tables))
	var sfnt []byte
	// Version: 00 01 00 00 (TrueType).
	sfnt = append(sfnt, 0x00, 0x01, 0x00, 0x00)
	sfnt = binary.BigEndian.AppendUint16(sfnt, numTables)
	// Search params (placeholders).
	sfnt = append(sfnt, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)

	// Table directory.
	tags := []string{"head", "name", "maxp", "OS/2"}
	dataOffset := 12 + int(numTables)*16
	for i, tbl := range tables {
		tag := tags[i]
		sfnt = append(sfnt, []byte(tag)...)
		sfnt = append(sfnt, 0, 0, 0, 0) // checksum placeholder.
		sfnt = binary.BigEndian.AppendUint32(sfnt, uint32(dataOffset))
		sfnt = binary.BigEndian.AppendUint32(sfnt, uint32(len(tbl)))
		dataOffset += len(tbl)
		// Pad to 4-byte boundary.
		dataOffset = (dataOffset + 3) &^ 3
	}

	// Table data.
	for _, tbl := range tables {
		sfnt = append(sfnt, tbl...)
		for len(sfnt)%4 != 0 {
			sfnt = append(sfnt, 0)
		}
	}

	return sfnt
}

func buildNameTable(family, subFamily, fullName string) []byte {
	// Platform 1 (Macintosh), encoding 0.
	names := []struct {
		nameID uint16
		value  string
	}{
		{1, family},
		{2, subFamily},
		{4, fullName},
	}

	var nameRecords []byte
	var stringData []byte
	stringOffset := uint16(0)

	for _, n := range names {
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, 1) // platformID.
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, 0) // encodingID.
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, 0) // languageID.
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, n.nameID)
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, uint16(len(n.value)))
		nameRecords = binary.BigEndian.AppendUint16(nameRecords, stringOffset)
		stringData = append(stringData, []byte(n.value)...)
		stringOffset += uint16(len(n.value))
	}

	var nameTable []byte
	nameTable = binary.BigEndian.AppendUint16(nameTable, 0)                          // format.
	nameTable = binary.BigEndian.AppendUint16(nameTable, uint16(len(names)))         // count.
	nameTable = binary.BigEndian.AppendUint16(nameTable, uint16(6+len(nameRecords))) // stringOffset.
	nameTable = append(nameTable, nameRecords...)
	nameTable = append(nameTable, stringData...)
	return nameTable
}

func TestAudioExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"mp3", "song.mp3", true},
		{"wav", "audio.wav", true},
		{"flac", "music.flac", true},
		{"ogg", "audio.ogg", true},
		{"aac", "audio.aac", true},
		{"wma", "audio.wma", true},
		{"aiff", "audio.aiff", true},
		{"m4a", "audio.m4a", true},
		{"opus", "audio.opus", true},
		{"uppercase", "SONG.MP3", true},
		{"go file", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, explorer.CanHandle(tt.path, nil))
		})
	}
}

func TestAudioExplorer_WAV(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}
	content := buildWAV(44100, 16, 2, 5*44100*2*2) // 5 seconds, 44.1kHz, 16-bit, stereo.

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.wav", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "audio", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Audio file: test.wav")
	require.Contains(t, result.Summary, "Format: WAV")
	require.Contains(t, result.Summary, "Sample rate: 44100 Hz")
	require.Contains(t, result.Summary, "Channels: 2 (stereo)")
	require.Contains(t, result.Summary, "Bits per sample: 16")
}

func TestAudioExplorer_FLAC(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}
	content := buildFLAC(48000, 2, 16, 1000)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "music.flac", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: FLAC")
	require.Contains(t, result.Summary, "Sample rate: 48000 Hz")
	require.Contains(t, result.Summary, "Channels: 2 (stereo)")
	require.Contains(t, result.Summary, "Bits per sample: 16")
}

func TestAudioExplorer_MP3(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}
	content := buildMP3(128)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "song.mp3", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: MP3")
	require.Contains(t, result.Summary, "Audio file: song.mp3")
}

func TestAudioExplorer_OGG(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}
	content := buildOGG()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "audio.ogg", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: OGG")
}

func TestAudioExplorer_ShortContent(t *testing.T) {
	t.Parallel()
	explorer := &AudioExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "short.wav", Content: []byte{0x00, 0x01},
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Audio file: short.wav")
}

func TestAudioExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	content := buildWAV(22050, 8, 1, 22050)

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path: "test.wav", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "audio", result.ExplorerUsed)
}

func TestAudioExplorer_FormatDuration(t *testing.T) {
	t.Parallel()
	require.Equal(t, "0:05", formatDuration(5.0))
	require.Equal(t, "1:30", formatDuration(90.0))
	require.Equal(t, "1:01:00", formatDuration(3660.0))
	require.Equal(t, "unknown", formatDuration(math.NaN()))
}

// buildWAV creates a minimal WAV file with the given parameters.
func buildWAV(sampleRate, bitsPerSample, channels, dataSize int) []byte {
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	var buf bytes.Buffer
	// RIFF header.
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")

	// fmt chunk.
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16)) // chunk size.
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM format.
	binary.Write(&buf, binary.LittleEndian, uint16(channels))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))

	// data chunk.
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	// Write minimal audio data.
	for i := 0; i < min(dataSize, 256); i++ {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// buildFLAC creates a minimal FLAC file with STREAMINFO.
func buildFLAC(sampleRate, channels, bitsPerSample int, totalSamples uint64) []byte {
	var buf bytes.Buffer
	buf.WriteString("fLaC")

	// STREAMINFO block (type=0, 34 bytes).
	streamInfo := make([]byte, 34)
	// minBlockSize(2) + maxBlockSize(2).
	binary.BigEndian.PutUint16(streamInfo[0:2], 4096)
	binary.BigEndian.PutUint16(streamInfo[2:4], 4096)
	// minFrameSize(3) + maxFrameSize(3) = 6 bytes (zeros).
	// sampleRate(20 bits) + channels(3 bits) + bps(5 bits) + totalSamples(36 bits).
	sr := uint32(sampleRate)
	ch := uint32(channels - 1)
	bps := uint32(bitsPerSample - 1)
	streamInfo[10] = byte(sr >> 12)
	streamInfo[11] = byte((sr >> 4) & 0xFF)
	streamInfo[12] = byte((sr&0x0F)<<4 | (ch<<1)&0x0E | (bps>>4)&0x01)
	streamInfo[13] = byte((bps&0x0F)<<4 | uint32(totalSamples>>32)&0x0F)
	streamInfo[14] = byte(totalSamples >> 24)
	streamInfo[15] = byte(totalSamples >> 16)
	streamInfo[16] = byte(totalSamples >> 8)
	streamInfo[17] = byte(totalSamples)
	// MD5(16 bytes) = zeros.

	// Block header: last(1) + type(0) + length(24).
	header := make([]byte, 4)
	header[0] = 0x80 // last block + type 0.
	binary.BigEndian.PutUint16(header[2:4], 34)

	buf.Write(header)
	buf.Write(streamInfo)
	// Add some padding to make the file non-trivial.
	buf.Write(make([]byte, 100))
	return buf.Bytes()
}

// buildMP3 creates a minimal MP3 file with ID3v2 tag and frames.
func buildMP3(bitrate int) []byte {
	var buf bytes.Buffer
	// ID3v2 header.
	buf.WriteString("ID3")
	buf.WriteByte(3) // version 2.3.
	buf.WriteByte(0) // revision.
	buf.WriteByte(0) // flags.
	buf.Write([]byte{0, 0, 0, 0})

	// Add MPEG1 Layer 3 frames at 44100Hz stereo.
	brIndex := mp3BitrateIndex(bitrate)
	for i := 0; i < 10; i++ {
		// Frame sync: 0xFF 0xFB (MPEG1, Layer 3, no CRC).
		buf.WriteByte(0xFF)
		buf.WriteByte(0xFB)
		// Byte 2: bitrate_index(4) | sample_rate_index(2) | padding(1) | private(1).
		// sample_rate_index=0 means 44100 for MPEG1.
		buf.WriteByte(byte(brIndex<<4) | 0x00)
		// Byte 3: channel_mode(2) | ... 0x00 = stereo.
		buf.WriteByte(0x00)
		// Frame payload: ~417 bytes for 128kbps @ 44100Hz.
		frameData := make([]byte, 413)
		buf.Write(frameData)
	}
	return buf.Bytes()
}

func mp3BitrateIndex(bitrate int) int {
	table := []int{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	for i, br := range table {
		if br == bitrate {
			return i + 1
		}
	}
	return 9 // Default to 128.
}

// buildOGG creates a minimal OGG file with Vorbis identification header.
func buildOGG() []byte {
	var buf bytes.Buffer
	// OGG page header.
	buf.WriteString("OggS")
	buf.WriteByte(0)                                   // version.
	buf.WriteByte(0)                                   // flags.
	binary.Write(&buf, binary.LittleEndian, uint64(0)) // granule.
	binary.Write(&buf, binary.LittleEndian, uint32(1)) // serial.
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // page seq.
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // checksum.
	buf.WriteByte(1)                                   // segments.
	buf.WriteByte(30)                                  // segment length.

	// Vorbis identification header.
	buf.WriteByte(0x01) // packet type: ident.
	buf.WriteString("vorbis")
	binary.Write(&buf, binary.LittleEndian, uint32(0))      // version.
	buf.WriteByte(2)                                        // channels.
	binary.Write(&buf, binary.LittleEndian, uint32(44100))  // sample rate.
	binary.Write(&buf, binary.LittleEndian, uint32(128000)) // bitrate max.
	binary.Write(&buf, binary.LittleEndian, uint32(128000)) // bitrate nominal.
	binary.Write(&buf, binary.LittleEndian, uint32(128000)) // bitrate min.
	buf.WriteByte(0)                                        // block sizes.
	buf.WriteByte(1)                                        // framing.

	return buf.Bytes()
}
