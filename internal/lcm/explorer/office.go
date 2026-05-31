package explorer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// OfficeExplorer explores Office document files (OOXML and ODF formats).
// Uses archive/zip + XML parsing. No external dependencies required.
type OfficeExplorer struct{}

// officeExtensions maps recognized Office document extensions to their sub-type.
var officeExtensions = map[string]string{
	"docx": "docx",
	"xlsx": "xlsx",
	"pptx": "pptx",
	"odt":  "odt",
	"ods":  "ods",
	"odp":  "odp",
	"doc":  "legacy",
	"xls":  "legacy",
	"ppt":  "legacy",
}

func (e *OfficeExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := officeExtensions[ext]; ok {
		return true
	}
	// Check ZIP-based office files by magic bytes (PK\x03\x04) + content type.
	if len(content) >= 4 && bytes.HasPrefix(content, []byte{0x50, 0x4B, 0x03, 0x04}) {
		return hasOfficeContentTypes(content)
	}
	return false
}

// hasOfficeContentTypes checks if a ZIP file contains Office content type info.
func hasOfficeContentTypes(content []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "[Content_Types].xml" {
			return true
		}
	}
	return false
}

func (e *OfficeExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	subType := officeExtensions[ext]
	name := filepath.Base(input.Path)

	var summary strings.Builder
	fmt.Fprintf(&summary, "Office document: %s\n", name)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	if subType == "legacy" {
		summary.WriteString("Format: Legacy binary Office format (OLE2)\n")
		summary.WriteString("Note: Legacy .doc/.xls/.ppt files have limited metadata extraction\n")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "office",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	// Parse as ZIP.
	r, err := zip.NewReader(bytes.NewReader(input.Content), int64(len(input.Content)))
	if err != nil {
		summary.WriteString("Error: unable to parse as ZIP archive\n")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "office",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	// Detect OOXML vs ODF.
	isOOXML := hasOfficeContentTypes(input.Content)

	if isOOXML {
		exploreOOXML(&summary, r, subType)
	} else {
		exploreODF(&summary, r, subType)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "office",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreOOXML extracts metadata from OOXML files (docx/xlsx/pptx).
func exploreOOXML(summary *strings.Builder, r *zip.Reader, subType string) {
	// Extract core properties.
	coreProps := readZipEntry(r, "docProps/core.xml")
	if len(coreProps) > 0 {
		extractOOXMLCoreProps(summary, coreProps)
	}

	// Format-specific extraction.
	switch subType {
	case "docx":
		exploreDocx(summary, r)
	case "xlsx":
		exploreXlsx(summary, r)
	case "pptx":
		explorePptx(summary, r)
	default:
		// Try format detection from content types.
		contentTypes := readZipEntry(r, "[Content_Types].xml")
		if bytes.Contains(contentTypes, []byte("wordprocessingml")) {
			exploreDocx(summary, r)
		} else if bytes.Contains(contentTypes, []byte("spreadsheetml")) {
			exploreXlsx(summary, r)
		} else if bytes.Contains(contentTypes, []byte("presentationml")) {
			explorePptx(summary, r)
		}
	}
}

// exploreDocx extracts DOCX-specific metadata.
func exploreDocx(summary *strings.Builder, r *zip.Reader) {
	summary.WriteString("\nDocument type: Word processing (DOCX)\n")

	// Count paragraphs in document.xml.
	docXML := readZipEntry(r, "word/document.xml")
	if len(docXML) > 0 {
		paraCount := strings.Count(string(docXML), "<w:p ")
		paraCount += strings.Count(string(docXML), "<w:p>")
		if paraCount > 0 {
			fmt.Fprintf(summary, "Paragraphs: %d\n", paraCount)
		}
	}

	// Extract app properties for page count.
	appProps := readZipEntry(r, "docProps/app.xml")
	if len(appProps) > 0 {
		if pages := extractXMLValue(appProps, "Pages"); pages != "" {
			fmt.Fprintf(summary, "Pages: %s\n", pages)
		}
		if words := extractXMLValue(appProps, "Words"); words != "" {
			fmt.Fprintf(summary, "Words: %s\n", words)
		}
		if chars := extractXMLValue(appProps, "Characters"); chars != "" {
			fmt.Fprintf(summary, "Characters: %s\n", chars)
		}
	}
}

// exploreXlsx extracts XLSX-specific metadata.
func exploreXlsx(summary *strings.Builder, r *zip.Reader) {
	summary.WriteString("\nDocument type: Spreadsheet (XLSX)\n")

	// Extract sheet names from workbook.xml.
	wbXML := readZipEntry(r, "xl/workbook.xml")
	if len(wbXML) > 0 {
		sheetNames := extractXMLValues(wbXML, "sheet", "name")
		if len(sheetNames) > 0 {
			fmt.Fprintf(summary, "Sheets (%d):\n", len(sheetNames))
			for i, name := range sheetNames {
				if i >= 20 {
					fmt.Fprintf(summary, "  ... and %d more sheets\n", len(sheetNames)-20)
					break
				}
				fmt.Fprintf(summary, "  %d. %s\n", i+1, name)
			}
		}
	}
}

// explorePptx extracts PPTX-specific metadata.
func explorePptx(summary *strings.Builder, r *zip.Reader) {
	summary.WriteString("\nDocument type: Presentation (PPTX)\n")

	// Count slides.
	slideCount := 0
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") &&
			strings.HasSuffix(f.Name, ".xml") {
			slideCount++
		}
	}
	if slideCount > 0 {
		fmt.Fprintf(summary, "Slides: %d\n", slideCount)
	}

	// Extract app properties.
	appProps := readZipEntry(r, "docProps/app.xml")
	if len(appProps) > 0 {
		if slides := extractXMLValue(appProps, "Slides"); slides != "" {
			fmt.Fprintf(summary, "Total slides (metadata): %s\n", slides)
		}
	}
}

// exploreODF extracts metadata from ODF files (odt/ods/odp).
func exploreODF(summary *strings.Builder, r *zip.Reader, subType string) {
	switch subType {
	case "odt":
		summary.WriteString("\nDocument type: Word processing (ODT)\n")
	case "ods":
		summary.WriteString("\nDocument type: Spreadsheet (ODS)\n")
	case "odp":
		summary.WriteString("\nDocument type: Presentation (ODP)\n")
	default:
		summary.WriteString("\nDocument type: OpenDocument Format\n")
	}

	// Extract meta.xml.
	metaXML := readZipEntry(r, "meta.xml")
	if len(metaXML) > 0 {
		extractODFMeta(summary, metaXML)
	}

	// Count sheets for ODS.
	if subType == "ods" {
		contentXML := readZipEntry(r, "content.xml")
		if len(contentXML) > 0 {
			sheetCount := strings.Count(string(contentXML), "<table:table ")
			if sheetCount > 0 {
				fmt.Fprintf(summary, "Sheets: %d\n", sheetCount)
			}
		}
	}
}

// extractOOXMLCoreProps extracts Dublin Core metadata from OOXML core.xml.
func extractOOXMLCoreProps(summary *strings.Builder, data []byte) {
	firstProp := true
	props := []struct {
		tag   string
		label string
	}{
		{"dc:title", "Title"},
		{"dc:creator", "Author"},
		{"dc:subject", "Subject"},
		{"dcterms:created", "Created"},
		{"dcterms:modified", "Modified"},
	}

	for _, p := range props {
		val := extractXMLValue(data, p.tag)
		if val == "" {
			continue
		}
		if firstProp {
			summary.WriteString("\nMetadata:\n")
			firstProp = false
		}
		fmt.Fprintf(summary, "  %s: %s\n", p.label, val)
	}
}

// extractODFMeta extracts metadata from ODF meta.xml.
func extractODFMeta(summary *strings.Builder, data []byte) {
	firstProp := true
	props := []struct {
		tag   string
		label string
	}{
		{"dc:title", "Title"},
		{"dc:creator", "Author"},
		{"dc:subject", "Subject"},
		{"dc:date", "Date"},
		{"meta:editing-duration", "Editing duration"},
		{"meta:document-statistic", ""},
	}

	for _, p := range props {
		if p.tag == "meta:document-statistic" {
			// Extract attributes from the statistic element.
			extractODFStats(summary, data, &firstProp)
			continue
		}
		val := extractXMLValue(data, p.tag)
		if val == "" {
			continue
		}
		if firstProp {
			summary.WriteString("\nMetadata:\n")
			firstProp = false
		}
		fmt.Fprintf(summary, "  %s: %s\n", p.label, val)
	}
}

// extractODFStats extracts document statistics from ODF meta.xml.
func extractODFStats(summary *strings.Builder, data []byte, firstProp *bool) {
	text := string(data)
	// Look for meta:document-statistic element attributes.
	attrs := []string{
		"meta:page-count", "meta:word-count", "meta:character-count",
		"meta:table-count", "meta:image-count", "meta:object-count",
	}
	labels := map[string]string{
		"meta:page-count":      "Pages",
		"meta:word-count":      "Words",
		"meta:character-count": "Characters",
		"meta:table-count":     "Tables",
		"meta:image-count":     "Images",
		"meta:object-count":    "Objects",
	}

	for _, attr := range attrs {
		val := extractXMLAttr(text, attr)
		if val == "" {
			continue
		}
		if *firstProp {
			summary.WriteString("\nMetadata:\n")
			*firstProp = false
		}
		fmt.Fprintf(summary, "  %s: %s\n", labels[attr], val)
	}
}

// readZipEntry reads a file from a zip.Reader and returns its contents.
// Returns nil if the file is not found or cannot be read.
func readZipEntry(r *zip.Reader, name string) []byte {
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil
			}
			return data
		}
	}
	return nil
}

// extractXMLValue extracts the text content of the first XML element with
// the given tag from data. It handles namespaced tags.
func extractXMLValue(data []byte, tag string) string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var depth int
	var inTarget bool

	for {
		tok, err := decoder.Token()
		if err != nil {
			return ""
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if matchesTag(se.Name.Local, tag) {
				inTarget = true
				depth = 1
			} else if inTarget {
				depth++
			}
		case xml.CharData:
			if inTarget && depth == 1 {
				return strings.TrimSpace(string(se))
			}
		case xml.EndElement:
			if inTarget {
				depth--
				if depth == 0 {
					inTarget = false
				}
			}
		}
	}
}

// extractXMLValues extracts attribute values from XML elements matching the
// given tag name. Returns values for the specified attribute.
func extractXMLValues(data []byte, tagName string, attrName string) []string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var results []string

	for {
		tok, err := decoder.Token()
		if err != nil {
			return results
		}
		if se, ok := tok.(xml.StartElement); ok {
			if matchesTag(se.Name.Local, tagName) {
				for _, attr := range se.Attr {
					if matchesTag(attr.Name.Local, attrName) {
						results = append(results, attr.Value)
					}
				}
			}
		}
	}
}

// extractXMLAttr extracts an attribute value from XML text using string
// matching. This is a lightweight alternative for ODF meta attributes.
func extractXMLAttr(text string, attr string) string {
	// Look for attr="value" pattern.
	prefix := attr + `="`
	start := strings.Index(text, prefix)
	if start < 0 {
		prefix = strings.ReplaceAll(attr, ":", ":") + `="`
		start = strings.Index(text, prefix)
	}
	if start < 0 {
		return ""
	}
	valueStart := start + len(prefix)
	end := strings.IndexByte(text[valueStart:], '"')
	if end < 0 {
		return ""
	}
	return text[valueStart : valueStart+end]
}

// matchesTag checks if a local XML name matches the simple tag name
// (after namespace prefix).
func matchesTag(localName, tag string) bool {
	// Handle namespaced tags like "dc:title" or just "title".
	if idx := strings.Index(tag, ":"); idx >= 0 {
		// Tag has namespace prefix. Match local name against suffix.
		return localName == tag[idx+1:] || localName == tag
	}
	return localName == tag
}
