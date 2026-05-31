package explorer

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

// FontExplorer explores font files (TrueType, OpenType, WOFF, WOFF2).
// Parses SFNT/OTF header tables directly. No external dependencies.
type FontExplorer struct{}

var fontExtensions = map[string]bool{
	"ttf":   true,
	"otf":   true,
	"woff":  true,
	"woff2": true,
	"ttc":   true,
	"eot":   true,
}

func (e *FontExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if fontExtensions[ext] {
		return true
	}
	return detectFontFormat(content) != ""
}

func (e *FontExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	format := detectFontFormat(input.Content)
	if format == "" {
		format = strings.ToUpper(ext)
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Font file: %s\n", name)
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	info := parseFontInfo(input.Content, format)
	if info.family != "" {
		fmt.Fprintf(&summary, "Family: %s\n", info.family)
	}
	if info.subFamily != "" {
		fmt.Fprintf(&summary, "Style: %s\n", info.subFamily)
	}
	if info.fullName != "" {
		fmt.Fprintf(&summary, "Full name: %s\n", info.fullName)
	}
	if info.weight > 0 {
		fmt.Fprintf(&summary, "Weight: %d (%s)\n", info.weight, fontWeightName(info.weight))
	}
	if info.italic {
		summary.WriteString("Italic: yes\n")
	}
	if info.unitsPerEm > 0 {
		fmt.Fprintf(&summary, "Units per em: %d\n", info.unitsPerEm)
	}
	if info.numGlyphs > 0 {
		fmt.Fprintf(&summary, "Glyphs: %d\n", info.numGlyphs)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "font",
		TokenEstimate: estimateTokens(result),
	}, nil
}

type fontInfo struct {
	family     string
	subFamily  string
	fullName   string
	weight     uint16
	italic     bool
	unitsPerEm uint16
	numGlyphs  uint16
	hasCmap    bool
	hasOS2     bool
}

func parseFontInfo(content []byte, format string) fontInfo {
	switch format {
	case "TrueType", "OpenType":
		return parseSFNT(content)
	case "WOFF":
		return parseWOFF(content)
	default:
		return fontInfo{}
	}
}

// parseSFNT parses TrueType/OpenType font data (SFNT container).
func parseSFNT(content []byte) fontInfo {
	if len(content) < 12 {
		return fontInfo{}
	}

	info := fontInfo{}
	numTables := binary.BigEndian.Uint16(content[4:6])

	// Walk table directory entries (16 bytes each starting at offset 12).
	for i := uint16(0); i < numTables; i++ {
		off := 12 + int(i)*16
		if off+16 > len(content) {
			break
		}
		tag := string(content[off : off+4])
		offset := binary.BigEndian.Uint32(content[off+8 : off+12])
		length := binary.BigEndian.Uint32(content[off+12 : off+16])

		switch tag {
		case "head":
			parseHeadTable(content, offset, length, &info)
		case "name":
			parseNameTable(content, offset, length, &info)
		case "maxp":
			parseMaxpTable(content, offset, length, &info)
		case "OS/2":
			info.hasOS2 = true
			parseOS2Table(content, offset, length, &info)
		case "cmap":
			info.hasCmap = true
		}
	}

	return info
}

// parseHeadTable extracts units per em from the 'head' table.
func parseHeadTable(content []byte, offset, length uint32, info *fontInfo) {
	// head table: offset 18 = unitsPerEm (uint16 BE).
	if offset+20 > uint32(len(content)) || length < 54 {
		return
	}
	info.unitsPerEm = binary.BigEndian.Uint16(content[offset+18 : offset+20])
}

// parseNameTable extracts family, sub-family, and full name from the 'name' table.
func parseNameTable(content []byte, offset, length uint32, info *fontInfo) {
	if offset+6 > uint32(len(content)) {
		return
	}
	start := int(offset)
	// name table header: format (2), count (2), stringOffset (2).
	nameCount := int(binary.BigEndian.Uint16(content[start+2 : start+4]))
	stringAreaOffset := start + int(binary.BigEndian.Uint16(content[start+4:start+6]))

	for i := 0; i < nameCount; i++ {
		recOff := start + 6 + i*12
		if recOff+12 > len(content) {
			break
		}
		platformID := binary.BigEndian.Uint16(content[recOff : recOff+2])
		_ = platformID
		nameID := binary.BigEndian.Uint16(content[recOff+6 : recOff+8])
		strLen := int(binary.BigEndian.Uint16(content[recOff+8 : recOff+10]))
		strOff := int(binary.BigEndian.Uint16(content[recOff+10 : recOff+12]))

		strStart := stringAreaOffset + strOff
		if strStart+strLen > len(content) {
			continue
		}
		raw := content[strStart : strStart+strLen]

		var value string
		if platformID == 0 || platformID == 3 {
			// Unicode: UTF-16BE.
			var sb strings.Builder
			for j := 0; j+1 < len(raw); j += 2 {
				r := rune(binary.BigEndian.Uint16(raw[j : j+2]))
				if r >= 0x20 && r < 0xFFFF {
					sb.WriteRune(r)
				}
			}
			value = sb.String()
		} else {
			value = string(raw)
		}

		switch nameID {
		case 1:
			if info.family == "" {
				info.family = value
			}
		case 2:
			if info.subFamily == "" {
				info.subFamily = value
			}
		case 4:
			if info.fullName == "" {
				info.fullName = value
			}
		}
	}
}

// parseMaxpTable extracts the glyph count from the 'maxp' table.
func parseMaxpTable(content []byte, offset, length uint32, info *fontInfo) {
	if offset+6 > uint32(len(content)) {
		return
	}
	info.numGlyphs = binary.BigEndian.Uint16(content[offset+4 : offset+6])
}

// parseOS2Table extracts weight and italic info from the OS/2 table.
func parseOS2Table(content []byte, offset, length uint32, info *fontInfo) {
	if offset+78 > uint32(len(content)) {
		return
	}
	// usWeightClass at offset 4 within OS/2 table.
	info.weight = binary.BigEndian.Uint16(content[offset+4 : offset+6])
	// fsSelection at offset 62: bit 0 = italic.
	fsSelection := binary.BigEndian.Uint16(content[offset+62 : offset+64])
	info.italic = fsSelection&1 != 0
}

// parseWOFF parses WOFF (Web Open Font Format) header and delegates to SFNT.
func parseWOFF(content []byte) fontInfo {
	if len(content) < 44 {
		return fontInfo{}
	}
	// WOFF header: signature (4) + flavor (4) + length (4) + numTables (2).
	numTables := binary.BigEndian.Uint16(content[12:14])

	// Reconstruct a minimal SFNT table directory to use parseSFNT logic.
	// WOFF table directory starts at offset 44, entries are 20 bytes each.
	var sfntHeader []byte
	// SFNT header: version (4) + numTables (2) + searchRange (2) + entrySelector (2) + rangeShift (2) = 12.
	sfntHeader = append(sfntHeader, content[4:8]...) // Copy flavor as version.
	sfntHeader = binary.BigEndian.AppendUint16(sfntHeader, numTables)
	// Placeholder search params.
	sfntHeader = append(sfntHeader, 0, 0, 0, 0, 0, 0)

	for i := uint16(0); i < numTables; i++ {
		woffOff := 44 + int(i)*20
		if woffOff+20 > len(content) {
			break
		}
		// WOFF entry: tag(4) + offset(4) + compLength(4) + origLength(4) + origChecksum(4).
		tag := content[woffOff : woffOff+4]
		woffOffset := binary.BigEndian.Uint32(content[woffOff+4 : woffOff+8])
		origLength := binary.BigEndian.Uint32(content[woffOff+12 : woffOff+16])

		// SFNT entry: tag(4) + checksum(4) + offset(4) + length(4) = 16 bytes.
		sfntHeader = append(sfntHeader, tag...)
		sfntHeader = append(sfntHeader, 0, 0, 0, 0) // checksum placeholder
		sfntHeader = binary.BigEndian.AppendUint32(sfntHeader, woffOffset)
		sfntHeader = binary.BigEndian.AppendUint32(sfntHeader, origLength)
	}

	return parseSFNT(sfntHeader)
}

// detectFontFormat returns the format name from magic bytes.
func detectFontFormat(content []byte) string {
	if len(content) < 4 {
		return ""
	}
	switch {
	case len(content) >= 4 && content[0] == 0x00 && content[1] == 0x01 && content[2] == 0x00 && content[3] == 0x00:
		return "TrueType"
	case len(content) >= 4 && string(content[:4]) == "OTTO":
		return "OpenType"
	case len(content) >= 4 && string(content[:4]) == "wOFF":
		return "WOFF"
	case len(content) >= 4 && string(content[:4]) == "wOF2":
		return "WOFF2"
	case len(content) >= 4 && string(content[:4]) == "ttcf":
		return "TTC"
	default:
		return ""
	}
}

// fontWeightName returns a human-readable name for a font weight value.
func fontWeightName(w uint16) string {
	switch {
	case w < 150:
		return "Thin"
	case w < 250:
		return "Extra-light"
	case w < 350:
		return "Light"
	case w < 450:
		return "Regular"
	case w < 550:
		return "Medium"
	case w < 650:
		return "Semi-bold"
	case w < 750:
		return "Bold"
	case w < 850:
		return "Extra-bold"
	default:
		return "Black"
	}
}
