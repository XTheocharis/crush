package explorer

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"mp4", "video.mp4", true},
		{"m4v", "video.m4v", true},
		{"mkv", "video.mkv", true},
		{"webm", "video.webm", true},
		{"avi", "video.avi", true},
		{"mov", "video.mov", true},
		{"wmv", "video.wmv", true},
		{"flv", "video.flv", true},
		{"uppercase", "VIDEO.MP4", true},
		{"go file", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, explorer.CanHandle(tt.path, nil))
		})
	}
}

func TestVideoExplorer_CanHandle_MagicBytes(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}

	// MP4 ftyp box.
	mp4Magic := buildMP4Ftyp()
	require.True(t, explorer.CanHandle("unknown", mp4Magic))

	// AVI RIFF.
	aviMagic := []byte("RIFF\x00\x00\x00\x00AVI ")
	require.True(t, explorer.CanHandle("unknown", aviMagic))

	// MKV EBML.
	mkvMagic := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	require.True(t, explorer.CanHandle("unknown", mkvMagic))

	// Random bytes.
	require.False(t, explorer.CanHandle("unknown", []byte{0x00, 0x01, 0x02, 0x03}))
}

func TestVideoExplorer_MP4(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}
	content := buildMP4(1920, 1080, "avc1")

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.mp4", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "video", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Video file: test.mp4")
	require.Contains(t, result.Summary, "Format: MP4")
}

func TestVideoExplorer_AVI(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}
	content := buildAVI(1280, 720, 30.0, "H264")

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.avi", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: AVI")
	require.Contains(t, result.Summary, "1280x720")
	require.Contains(t, result.Summary, "H.264")
}

func TestVideoExplorer_MKV(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}
	content := buildMKV(3840, 2160, "V_MPEG4/ISO/AVC")

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.mkv", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: MKV")
}

func TestVideoExplorer_ShortContent(t *testing.T) {
	t.Parallel()
	explorer := &VideoExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "short.mp4", Content: []byte{0x00, 0x01},
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Video file: short.mp4")
}

func TestVideoExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	content := buildMP4(640, 480, "avc1")

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path: "test.mp4", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "video", result.ExplorerUsed)
}

// buildMP4Ftyp creates minimal MP4 ftyp box.
func buildMP4Ftyp() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(12)) // box size.
	buf.WriteString("ftyp")
	buf.WriteString("isom")
	binary.Write(&buf, binary.BigEndian, uint32(0x200))
	return buf.Bytes()
}

// buildMP4 creates a minimal MP4 with moov/mvhd/tkhd/stsd boxes.
func buildMP4(width, height int, codec string) []byte {
	var buf bytes.Buffer

	// ftyp box.
	buf.Write(buildMP4Ftyp())

	// mvhd (version 0): 100 bytes.
	mvhdData := make([]byte, 100)
	timeScale := uint32(1000)
	totalDuration := uint32(60000) // 60 seconds.
	binary.BigEndian.PutUint32(mvhdData[12:16], timeScale)
	binary.BigEndian.PutUint32(mvhdData[16:20], totalDuration)
	mvhdBox := makeBox("mvhd", mvhdData)

	// tkhd (version 0): 84 bytes.
	tkhdData := make([]byte, 84)
	widthFixed := uint32(float64(width) * 65536.0)
	heightFixed := uint32(float64(height) * 65536.0)
	binary.BigEndian.PutUint32(tkhdData[76:80], widthFixed)
	binary.BigEndian.PutUint32(tkhdData[80:84], heightFixed)
	tkhdBox := makeBox("tkhd", tkhdData)

	// stsd with codec entry.
	var stsdPayload bytes.Buffer
	binary.Write(&stsdPayload, binary.BigEndian, uint32(1)) // entry count.
	entry := make([]byte, 16)
	copy(entry[4:8], codec)
	stsdPayload.Write(entry)
	stsdBox := makeBox("stsd", stsdPayload.Bytes())

	// Build box hierarchy: stbl > minf > mdia > trak.
	stblBox := makeBox("stbl", stsdBox)
	minfBox := makeBox("minf", stblBox)
	mdiaBox := makeBox("mdia", minfBox)
	var trakPayload bytes.Buffer
	trakPayload.Write(tkhdBox)
	trakPayload.Write(mdiaBox)
	trakBox := makeBox("trak", trakPayload.Bytes())

	// moov box.
	var moovPayload bytes.Buffer
	moovPayload.Write(mvhdBox)
	moovPayload.Write(trakBox)
	moovBox := makeBox("moov", moovPayload.Bytes())

	buf.Write(moovBox)
	return buf.Bytes()
}

func makeBox(boxType string, data []byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(8+len(data)))
	buf.WriteString(boxType)
	buf.Write(data)
	return buf.Bytes()
}

// buildAVI creates a minimal AVI RIFF structure.
func buildAVI(width, height int, fps float64, codec string) []byte {
	var hdrl bytes.Buffer

	// avih (main AVI header).
	avihData := make([]byte, 56)
	microsecPerFrame := uint32(1_000_000.0 / fps)
	totalFrames := uint32(300)
	binary.LittleEndian.PutUint32(avihData[0:4], microsecPerFrame)
	binary.LittleEndian.PutUint32(avihData[16:20], totalFrames)
	hdrl.Write(makeRIFFChunk("avih", avihData))

	// strh (stream header).
	strhData := make([]byte, 56)
	copy(strhData[0:4], "vids")
	copy(strhData[4:8], codec)
	hdrl.Write(makeRIFFChunk("strh", strhData))

	// strf (stream format — BITMAPINFOHEADER).
	strfData := make([]byte, 40)
	binary.LittleEndian.PutUint32(strfData[0:4], 40)
	binary.LittleEndian.PutUint32(strfData[4:8], uint32(width))
	binary.LittleEndian.PutUint32(strfData[8:12], uint32(height))
	hdrl.Write(makeRIFFChunk("strf", strfData))

	// Build hdrl LIST.
	var hdrlList bytes.Buffer
	hdrlList.WriteString("hdrl")
	hdrlList.Write(hdrl.Bytes())
	hdrlListChunk := makeRIFFChunk("LIST", hdrlList.Bytes())

	// Build full RIFF.
	var riff bytes.Buffer
	riff.WriteString("AVI ")
	riff.Write(hdrlListChunk)

	return makeRIFFChunk("RIFF", riff.Bytes())
}

func makeRIFFChunk(id string, data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(id)
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	return buf.Bytes()
}

// buildMKV creates a minimal Matroska file with EBML header and tracks.
func buildMKV(width, height int, codecID string) []byte {
	var buf bytes.Buffer

	// EBML header.
	buf.Write([]byte{0x1A, 0x45, 0xDF, 0xA3})                   // EBML element ID.
	buf.Write([]byte{0x93})                                     // data size = 19 (VINT).
	buf.Write([]byte{0x42, 0x86, 0x81, 0x01})                   // EBMLVersion = 1.
	buf.Write([]byte{0x42, 0xF7, 0x81, 0x01})                   // EBMLReadVersion = 1.
	buf.Write([]byte{0x42, 0xF2, 0x81, 0x04})                   // EBMLMaxIDLength = 4.
	buf.Write([]byte{0x42, 0xF3, 0x81, 0x08})                   // EBMLMaxSizeLength = 8.
	buf.Write([]byte{0x42, 0x82, 0x84, 0x77, 0x65, 0x62, 0x6D}) // DocType = "webM".

	// Segment (simplified — just enough for parser).
	buf.Write([]byte{0x18, 0x53, 0x80, 0x67})                         // Segment ID.
	buf.Write([]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00}) // Unknown size.

	// Tracks element.
	var tracks bytes.Buffer

	// TrackEntry.
	var trackEntry bytes.Buffer

	// CodecID.
	trackEntry.Write([]byte{0x86}) // CodecID.
	trackEntry.Write(append([]byte{byte(0x80 | len(codecID))}, []byte(codecID)...))

	// Video element.
	var video bytes.Buffer

	// PixelWidth.
	video.Write([]byte{0xB0})
	video.Write(append([]byte{0x81}, byte(width)))

	// PixelHeight.
	video.Write([]byte{0xBA})
	video.Write(append([]byte{0x81}, byte(height)))

	trackEntry.Write(append([]byte{0xE0, 0x80 | byte(len(video.Bytes()))}, video.Bytes()...))
	tracks.Write(append([]byte{0xAE, 0x80 | byte(len(trackEntry.Bytes()))}, trackEntry.Bytes()...))

	buf.Write(append([]byte{0x16, 0x54, 0xAE, 0x6B, 0x80 | byte(len(tracks.Bytes()))}, tracks.Bytes()...))

	return buf.Bytes()
}

func TestDiagramExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"drawio", "diagram.drawio", true},
		{"dio", "diagram.dio", true},
		{"mermaid", "diagram.mermaid", true},
		{"mmd", "diagram.mmd", true},
		{"d2", "diagram.d2", true},
		{"excalidraw", "diagram.excalidraw", true},
		{"plantuml", "diagram.plantuml", true},
		{"puml", "diagram.puml", true},
		{"uppercase", "DIAGRAM.DRAWIO", true},
		{"go file", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, explorer.CanHandle(tt.path, nil))
		})
	}
}

func TestDiagramExplorer_Drawio(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	content := []byte(`<mxfile><diagram><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" value="Node A" style="rounded=1" vertex="1" parent="1"/><mxCell id="3" value="Node B" style="ellipse" vertex="1" parent="1"/><mxCell id="4" value="" edge="1" source="2" target="3" parent="1"/></root></mxGraphModel></diagram></mxfile>`)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.drawio", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "diagram", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Diagram file: test.drawio")
	require.Contains(t, result.Summary, "Format: drawio")
	require.Contains(t, result.Summary, "Nodes: 2")
	require.Contains(t, result.Summary, "Edges (connections): 1")
	require.Contains(t, result.Summary, "Shape types:")
}

func TestDiagramExplorer_Mermaid(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	content := []byte(`graph TD
    A[Start] --> B{Decision}
    B -->|Yes| C[Action]
    B -->|No| D[End]
    C --> D`)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "flow.mmd", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Diagram type: flowchart")
	require.Contains(t, result.Summary, "Nodes:")
	require.Contains(t, result.Summary, "Edges:")
}

func TestDiagramExplorer_D2(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	content := []byte(`# My diagram
x: A box
y: Another box
x -> y: Connection
style x: fill: blue`)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "test.d2", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: d2")
	require.Contains(t, result.Summary, "Connections: 1")
}

func TestDiagramExplorer_Excalidraw(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	content := []byte(`{
		"elements": [
			{"type": "rectangle", "x": 0, "y": 0},
			{"type": "rectangle", "x": 100, "y": 100},
			{"type": "arrow", "x": 50, "y": 50},
			{"type": "text", "x": 200, "y": 200}
		],
		"appState": {}
	}`)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "drawing.excalidraw", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: excalidraw")
	require.Contains(t, result.Summary, "Elements: 4")
	require.Contains(t, result.Summary, "rectangle: 2")
	require.Contains(t, result.Summary, "arrow: 1")
	require.Contains(t, result.Summary, "text: 1")
}

func TestDiagramExplorer_PlantUML(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	content := []byte(`@startuml
Alice -> Bob: Hello
Bob --> Alice: Hi
@enduml`)

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "seq.puml", Content: content,
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Format: plantuml")
	require.Contains(t, result.Summary, "Diagram type: class")
}

func TestDiagramExplorer_EmptyFile(t *testing.T) {
	t.Parallel()
	explorer := &DiagramExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path: "empty.mmd", Content: []byte(""),
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Diagram file: empty.mmd")
}

func TestDiagramExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	content := []byte("graph TD\n    A --> B\n    B --> C")

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path: "test.mmd", Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "diagram", result.ExplorerUsed)
}
