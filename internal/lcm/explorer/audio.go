package explorer

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// AudioExplorer explores audio files (MP3, WAV, FLAC, OGG).
// Parses binary headers directly. No external dependencies.
type AudioExplorer struct{}

var audioExtensions = map[string]bool{
	"mp3":  true,
	"wav":  true,
	"flac": true,
	"ogg":  true,
	"aac":  true,
	"wma":  true,
	"aiff": true,
	"m4a":  true,
	"opus": true,
}

func (e *AudioExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if audioExtensions[ext] {
		return true
	}
	return detectAudioFormat(content) != ""
}

func (e *AudioExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	format := detectAudioFormat(input.Content)
	if format == "" {
		format = strings.ToUpper(ext)
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Audio file: %s\n", name)
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	info := parseAudioInfo(input.Content, format)

	if info.duration > 0 {
		fmt.Fprintf(&summary, "Duration: %s\n", formatDuration(info.duration))
	}
	if info.bitrate > 0 {
		fmt.Fprintf(&summary, "Bitrate: %d kbps\n", info.bitrate)
	}
	if info.sampleRate > 0 {
		fmt.Fprintf(&summary, "Sample rate: %d Hz\n", info.sampleRate)
	}
	if info.channels > 0 {
		fmt.Fprintf(&summary, "Channels: %d (%s)\n", info.channels, channelName(info.channels))
	}
	if info.bitsPerSample > 0 {
		fmt.Fprintf(&summary, "Bits per sample: %d\n", info.bitsPerSample)
	}

	// MP3 ID3 tag extraction.
	if format == "MP3" {
		extractID3Tags(&summary, input.Content)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "audio",
		TokenEstimate: estimateTokens(result),
	}, nil
}

type audioInfo struct {
	duration      float64 // seconds
	bitrate       int     // kbps
	sampleRate    int     // Hz
	channels      int
	bitsPerSample int
}

func parseAudioInfo(content []byte, format string) audioInfo {
	switch format {
	case "WAV":
		return parseWAV(content)
	case "FLAC":
		return parseFLAC(content)
	case "MP3":
		return parseMP3(content)
	case "OGG":
		return parseOGG(content)
	default:
		return audioInfo{}
	}
}

// parseWAV parses RIFF/WAV header for audio metadata.
func parseWAV(content []byte) audioInfo {
	// RIFF header: "RIFF" (4) + size (4) + "WAVE" (4) = 12 bytes minimum.
	if len(content) < 44 {
		return audioInfo{}
	}
	if string(content[:4]) != "RIFF" || string(content[8:12]) != "WAVE" {
		return audioInfo{}
	}

	info := audioInfo{}

	// Walk chunks to find "fmt ".
	offset := 12
	for offset+8 <= len(content) {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))

		if chunkID == "fmt " {
			if offset+24 > len(content) {
				break
			}
			// fmt chunk data: audioFormat(2) + numChannels(2) + sampleRate(4) +
			// byteRate(4) + blockAlign(2) + bitsPerSample(2).
			audioFormat := binary.LittleEndian.Uint16(content[offset+8 : offset+10])
			info.channels = int(binary.LittleEndian.Uint16(content[offset+10 : offset+12]))
			info.sampleRate = int(binary.LittleEndian.Uint32(content[offset+12 : offset+16]))
			byteRate := int(binary.LittleEndian.Uint32(content[offset+16 : offset+20]))
			info.bitsPerSample = int(binary.LittleEndian.Uint16(content[offset+22 : offset+24]))

			if audioFormat != 0 {
				info.bitrate = byteRate * 8 / 1000
			}

			// Calculate duration from data chunk.
			dataOffset := findWAVDataChunk(content)
			if dataOffset > 0 && dataOffset+8 <= len(content) {
				dataSize := int64(binary.LittleEndian.Uint32(content[dataOffset+4 : dataOffset+8]))
				if byteRate > 0 {
					info.duration = float64(dataSize) / float64(byteRate)
				}
			}
			break
		}

		offset += 8 + chunkSize
		if chunkSize == 0 {
			break
		}
	}

	return info
}

// findWAVDataChunk finds the offset of the "data" chunk in WAV content.
func findWAVDataChunk(content []byte) int {
	offset := 12
	for offset+8 <= len(content) {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		if chunkID == "data" {
			return offset
		}
		offset += 8 + chunkSize
		if chunkSize == 0 {
			break
		}
	}
	return -1
}

// parseFLAC parses FLAC stream header for audio metadata.
func parseFLAC(content []byte) audioInfo {
	// FLAC stream: "fLaC" (4) + metadata blocks.
	if len(content) < 42 {
		return audioInfo{}
	}
	if string(content[:4]) != "fLaC" {
		return audioInfo{}
	}

	info := audioInfo{}
	offset := 4

	for offset+4 <= len(content) {
		// Metadata block header: last(1 bit) + type(7 bits) + length(24 bits).
		blockType := content[offset] & 0x7F
		blockLength := int(content[offset+1])<<16 | int(content[offset+2])<<8 | int(content[offset+3])

		if blockType == 0 {
			// STREAMINFO block: 34 bytes.
			dataStart := offset + 4
			if dataStart+34 > len(content) {
				break
			}
			data := content[dataStart:]
			info.sampleRate = int(uint32(data[10])<<12 | uint32(data[11])<<4 | uint32(data[12])>>4)
			info.channels = int(data[12]&0x0E>>1) + 1
			info.bitsPerSample = int(data[12]&0x01<<4|data[13]>>4) + 1
			totalSamples := uint64(data[13]&0x0F)<<32 | uint64(data[14])<<24 | uint64(data[15])<<16 |
				uint64(data[16])<<8 | uint64(data[17])
			if info.sampleRate > 0 {
				info.duration = float64(totalSamples) / float64(info.sampleRate)
			}
			if info.duration > 0 {
				fileSize := len(content)
				info.bitrate = int(float64(fileSize)*8/info.duration) / 1000
			}
		}

		isLast := content[offset]&0x80 != 0
		offset += 4 + blockLength
		if isLast {
			break
		}
	}

	return info
}

// parseMP3 parses MP3 frame headers for audio metadata.
func parseMP3(content []byte) audioInfo {
	info := audioInfo{}

	// Skip ID3v2 tag if present.
	start := 0
	if len(content) >= 10 && string(content[:3]) == "ID3" {
		tagSize := (int(content[6])&0x7F)<<21 | (int(content[7])&0x7F)<<14 |
			(int(content[8])&0x7F)<<7 | (int(content[9]) & 0x7F)
		start = 10 + tagSize
	}

	// Find first valid MPEG frame sync (0xFF + 0Ex).
	frameCount := 0
	totalFrameSize := 0
	offset := start

	for offset+4 <= len(content) && frameCount < 1000 {
		if content[offset] != 0xFF {
			offset++
			continue
		}
		if content[offset+1]&0xE0 != 0xE0 {
			offset++
			continue
		}

		frameHdr := binary.BigEndian.Uint32(content[offset : offset+4])
		sr, ch, _, frameSize, valid := parseMPEGFrame(frameHdr)
		if !valid {
			offset++
			continue
		}

		if frameCount == 0 {
			info.sampleRate = sr
			info.channels = ch
		}
		totalFrameSize += frameSize
		frameCount++
		offset += frameSize

		// Stop at padding/garbage.
		if offset < len(content) && content[offset] == 0x00 {
			break
		}
	}

	if frameCount > 0 {
		totalSamples := frameCount * 1152
		if info.sampleRate > 0 {
			info.duration = float64(totalSamples) / float64(info.sampleRate)
		}
		if info.duration > 0 && totalFrameSize > 0 {
			info.bitrate = int(float64(totalFrameSize)*8/info.duration) / 1000
		}
	}

	return info
}

// parseMPEGFrame extracts sample rate, channels, bitrate, and frame size
// from a 32-bit MPEG audio frame header.
func parseMPEGFrame(hdr uint32) (sampleRate, channels, bitrate, frameSize int, valid bool) {
	// MPEG Audio version (bits 19-20).
	versionBits := (hdr >> 19) & 0x03
	if versionBits == 1 {
		return 0, 0, 0, 0, false // Reserved.
	}

	// Layer (bits 17-18).
	layerBits := (hdr >> 17) & 0x03
	if layerBits == 0 {
		return 0, 0, 0, 0, false // Reserved.
	}

	// Bitrate index (bits 12-15).
	bitrateIndex := (hdr >> 12) & 0x0F
	if bitrateIndex == 0 || bitrateIndex == 0x0F {
		return 0, 0, 0, 0, false
	}

	// Sample rate index (bits 10-11).
	srIndex := (hdr >> 10) & 0x03
	if srIndex == 0x03 {
		return 0, 0, 0, 0, false
	}

	// Compute values based on version and layer.
	var mpegVersion int
	switch versionBits {
	case 3:
		mpegVersion = 1
	case 2:
		mpegVersion = 2
	case 0:
		mpegVersion = 2 // MPEG 2.5.
	}

	var layer int
	switch layerBits {
	case 3:
		layer = 1
	case 2:
		layer = 2
	case 1:
		layer = 3
	}

	// Bitrate table (kbps).
	bitrateTable := [][16]int{
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0}, // V1 L1.
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},    // V1 L2.
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},     // V1 L3.
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},    // V2 L1.
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},         // V2 L2/L3.
	}

	tableIdx := 0
	if mpegVersion == 1 {
		switch layer {
		case 1:
			tableIdx = 0
		case 2:
			tableIdx = 1
		case 3:
			tableIdx = 2
		}
	} else {
		if layer == 1 {
			tableIdx = 3
		} else {
			tableIdx = 4
		}
	}

	bitrate = bitrateTable[tableIdx][bitrateIndex]

	// Sample rate table.
	srTable := [4][3]int{
		{44100, 48000, 32000}, // MPEG1.
		{22050, 24000, 16000}, // MPEG2.
		{11025, 12000, 8000},  // MPEG2.5.
		{0, 0, 0},
	}
	srVersion := 0
	if mpegVersion == 2 {
		srVersion = 1
	} else if mpegVersion == 2 && versionBits == 0 {
		srVersion = 2
	}
	sampleRate = srTable[srVersion][srIndex]

	// Channels (bit 6: 0=stereo, 1=joint/mono).
	channels = 2
	if (hdr>>6)&1 != 0 && layer == 3 {
		channels = 1
	}

	// Frame size.
	padding := int((hdr >> 9) & 1)
	if layer == 1 {
		frameSize = (12*bitrate*1000/sampleRate + padding) * 4
	} else {
		samplesPerFrame := 1152
		if mpegVersion != 1 && layer == 3 {
			samplesPerFrame = 576
		}
		frameSize = samplesPerFrame/8*bitrate*1000/sampleRate + padding
	}

	return sampleRate, channels, bitrate, frameSize, true
}

// parseOGG parses OGG Vorbis header for audio metadata.
func parseOGG(content []byte) audioInfo {
	// OGG page: "OggS" (4) + version(1) + ...
	if len(content) < 28 {
		return audioInfo{}
	}
	if string(content[:4]) != "OggS" {
		return audioInfo{}
	}

	info := audioInfo{}

	// Vorbis identification header starts in the second page typically.
	// Look for Vorbis start: 0x01 + "vorbis".
	for i := 0; i+30 <= len(content) && i < 4096; i++ {
		if content[i] == 0x01 && i+7 <= len(content) && string(content[i+1:i+7]) == "vorbis" {
			// Vorbis ident header: packet_type(1) + "vorbis"(6) + version(4) +
			// channels(1) + sampleRate(4).
			if i+16 <= len(content) {
				info.channels = int(content[i+11])
				info.sampleRate = int(binary.LittleEndian.Uint32(content[i+12 : i+16]))
				if i+24 <= len(content) {
					maxBitrate := int(binary.LittleEndian.Uint32(content[i+16 : i+20]))
					nomBitrate := int(binary.LittleEndian.Uint32(content[i+20 : i+24]))
					_ = maxBitrate
					if nomBitrate > 0 {
						info.bitrate = nomBitrate / 1000
					}
				}
			}
			break
		}
	}

	// Estimate duration from file size and bitrate.
	if info.bitrate > 0 {
		info.duration = float64(len(content)) * 8.0 / float64(info.bitrate*1000)
	}

	return info
}

// extractID3Tags extracts ID3v2 text frames as metadata.
func extractID3Tags(summary *strings.Builder, content []byte) {
	if len(content) < 10 || string(content[:3]) != "ID3" {
		return
	}

	// ID3v2 header: "ID3" (3) + version (2) + flags (1) + size (4 synchsafe).
	tagSize := (int(content[6])&0x7F)<<21 | (int(content[7])&0x7F)<<14 |
		(int(content[8])&0x7F)<<7 | (int(content[9]) & 0x7F)

	end := 10 + tagSize
	if end > len(content) {
		end = len(content)
	}

	frameLabels := map[string]string{
		"TIT2": "Title",
		"TPE1": "Artist",
		"TALB": "Album",
		"TYER": "Year",
		"TCON": "Genre",
		"TRCK": "Track",
	}

	var found bool
	offset := 10
	for offset+10 <= end {
		frameID := string(content[offset : offset+4])
		frameSize := int(binary.BigEndian.Uint32(content[offset+4 : offset+8]))
		if frameSize == 0 || frameSize > end {
			break
		}
		if label, ok := frameLabels[frameID]; ok && offset+10+frameSize <= end {
			if !found {
				summary.WriteString("\nID3 Tags:\n")
				found = true
			}
			// Skip encoding byte (1 byte) and read text.
			textStart := offset + 10 + 1
			textEnd := offset + 10 + frameSize
			if textEnd <= end && textStart < textEnd {
				text := sanitizeText(content[textStart:textEnd])
				if text != "" {
					fmt.Fprintf(summary, "  %s: %s\n", label, text)
				}
			}
		}
		offset += 10 + frameSize
	}
}

// detectAudioFormat returns the format name from magic bytes.
func detectAudioFormat(content []byte) string {
	if len(content) < 4 {
		return ""
	}
	switch {
	case string(content[:4]) == "RIFF" && len(content) >= 12 && string(content[8:12]) == "WAVE":
		return "WAV"
	case string(content[:4]) == "fLaC":
		return "FLAC"
	case len(content) >= 3 && content[0] == 0xFF && content[1]&0xE0 == 0xE0:
		return "MP3"
	case string(content[:3]) == "ID3":
		return "MP3"
	case string(content[:4]) == "OggS":
		return "OGG"
	default:
		return ""
	}
}

// formatDuration formats seconds as mm:ss or hh:mm:ss.
func formatDuration(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return "unknown"
	}
	totalSec := int(seconds)
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// channelName returns a descriptive name for channel count.
func channelName(ch int) string {
	switch ch {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1 surround"
	default:
		return fmt.Sprintf("%d channels", ch)
	}
}

// sanitizeText removes null bytes and non-printable characters.
func sanitizeText(data []byte) string {
	var sb strings.Builder
	for _, b := range data {
		if b >= 0x20 && b < 0x7F {
			sb.WriteByte(b)
		} else if b == 0 {
			break
		} else if b >= 0xC0 {
			// Likely UTF-8 lead byte — include raw.
			sb.WriteByte(b)
		}
	}
	return strings.TrimSpace(sb.String())
}
