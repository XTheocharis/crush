package explorer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// ArchiveExplorer explores archive and compressed file formats.
type ArchiveExplorer struct {
	formatterProfile OutputProfile
}

// archiveExtensions maps extensions to archive family identifiers.
var archiveExtensions = map[string]string{
	"zip":   "zip",
	"jar":   "zip",
	"war":   "zip",
	"ear":   "zip",
	"apk":   "zip",
	"ipa":   "zip",
	"nupkg": "zip",
	"crx":   "zip",
	"xpi":   "zip",
	"vsix":  "zip",
	"tar":   "tar",
	"gz":    "gzip",
	"tgz":   "tar.gz",
	"bz2":   "bzip2",
	"xz":    "xz",
	"lz":    "lz",
	"lz4":   "lz4",
	"zst":   "zstd",
	"7z":    "7z",
	"rar":   "rar",
	"cab":   "cab",
	"ar":    "ar",
	"deb":   "deb",
	"rpm":   "rpm",
	"cpio":  "cpio",
	"iso":   "iso",
	"dmg":   "dmg",
	"wim":   "wim",
}

// doubleExtensions maps compound extensions to archive families.
var doubleExtensions = map[string]string{
	".tar.gz":  "tar.gz",
	".tar.bz2": "tar.bz2",
	".tar.xz":  "tar.xz",
	".tar.lz":  "tar.lz",
	".tar.lz4": "tar.lz4",
	".tar.zst": "tar.zst",
}

// magicSignature describes a magic byte pattern for format identification.
type magicSignature struct {
	name   string
	family string
	offset int
	magic  []byte
}

// magicSignatures lists known archive magic byte signatures.
var magicSignatures = []magicSignature{
	{name: "ZIP", family: "zip", offset: 0, magic: []byte("PK\x03\x04")},
	{name: "gzip", family: "gzip", offset: 0, magic: []byte{0x1f, 0x8b}},
	{name: "RAR", family: "rar", offset: 0, magic: []byte("Rar!\x1a\x07")},
	{name: "7z", family: "7z", offset: 0, magic: []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}},
	{name: "XZ", family: "xz", offset: 0, magic: []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}},
	{name: "bzip2", family: "bzip2", offset: 0, magic: []byte("BZ")},
	{name: "LZ4", family: "lz4", offset: 0, magic: []byte{0x04, 0x22, 0x4d, 0x18}},
	{name: "zstd", family: "zstd", offset: 0, magic: []byte{0x28, 0xb5, 0x2f, 0xfd}},
	{name: "deb/ar", family: "deb", offset: 0, magic: []byte("!<arch>\n")},
	{name: "RPM", family: "rpm", offset: 0, magic: []byte{0xed, 0xab, 0xee, 0xdb}},
	{name: "TAR", family: "tar", offset: 257, magic: []byte("ustar")},
}

func (e *ArchiveExplorer) CanHandle(path string, content []byte) bool {
	// Check double extensions first (e.g., .tar.gz).
	lower := strings.ToLower(path)
	for ext := range doubleExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}

	// Check single extension.
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := archiveExtensions[ext]; ok {
		return true
	}

	// Fall back to magic byte detection.
	return detectArchiveMagic(content) != ""
}

// detectArchiveMagic returns the family name if magic bytes match, or empty.
func detectArchiveMagic(content []byte) string {
	for _, sig := range magicSignatures {
		end := sig.offset + len(sig.magic)
		if len(content) >= end &&
			bytes.Equal(content[sig.offset:end], sig.magic) {
			return sig.family
		}
	}
	return ""
}

func (e *ArchiveExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	family := e.resolveFamily(input.Path, input.Content)

	switch family {
	case "zip", "jar", "war", "ear", "apk", "ipa", "nupkg", "crx", "xpi", "vsix":
		return e.exploreZIP(input, family)
	case "tar":
		return e.exploreTAR(input, nil)
	case "tar.gz":
		return e.exploreTARCompressed(input, "gzip")
	case "tar.bz2":
		return e.exploreTARCompressed(input, "bzip2")
	case "tar.zst":
		return e.exploreTARCompressed(input, "zstd")
	case "gzip":
		// Standalone gzip could be a tar.gz; try tar first.
		return e.exploreGzip(input)
	case "bzip2":
		// Standalone bzip2 could be a tar.bz2; try tar first.
		return e.exploreBzip2(input)
	case "zstd":
		// Standalone zstd could be a tar.zst; try tar first.
		return e.exploreZstd(input)
	case "deb":
		return e.exploreDeb(input)
	case "ar":
		return e.exploreDeb(input) // ar format same as deb
	case "rpm":
		return e.exploreRPM(input)
	default:
		// Opaque formats: 7z, rar, xz, lz, lz4, cab, cpio, iso, dmg, wim.
		return e.exploreOpaque(input, family)
	}
}

// resolveFamily determines the archive family from path and content.
func (e *ArchiveExplorer) resolveFamily(path string, content []byte) string {
	lower := strings.ToLower(path)

	// Check double extensions first.
	for ext, family := range doubleExtensions {
		if strings.HasSuffix(lower, ext) {
			return family
		}
	}

	// Check single extension.
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if family, ok := archiveExtensions[ext]; ok {
		// For zip-family containers, return the specific extension so
		// we know to look for manifests etc.
		switch ext {
		case "jar", "war", "ear", "apk", "ipa", "nupkg", "crx", "xpi", "vsix":
			return ext
		}
		return family
	}

	// Magic byte fallback.
	return detectArchiveMagic(content)
}

// exploreZIP explores ZIP-family archives using pure Go archive/zip.
func (e *ArchiveExplorer) exploreZIP(input ExploreInput, family string) (ExploreResult, error) {
	reader, err := zip.NewReader(bytes.NewReader(input.Content), int64(len(input.Content)))
	if err != nil {
		summary := fmt.Sprintf("Archive file: %s\nFormat: %s\nSize: %d bytes\nError: could not read ZIP contents: %v",
			filepath.Base(input.Path), family, len(input.Content), err)
		return ExploreResult{
			Summary:       summary,
			ExplorerUsed:  "archive",
			TokenEstimate: estimateTokens(summary),
		}, nil
	}

	var (
		fileCount       int
		dirCount        int
		totalUncomp     uint64
		totalComp       uint64
		encrypted       bool
		extHist         = make(map[string]int)
		topLevel        = make(map[string]bool)
		comprMethods    = make(map[string]int)
		largest         []zipFileInfo
		manifestContent string
		minTime         time.Time
		maxTime         time.Time
		timeSet         bool
	)

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			dirCount++
			// Record top-level directory.
			parts := strings.SplitN(f.Name, "/", 2)
			if len(parts) > 0 && parts[0] != "" {
				topLevel[parts[0]+"/"] = true
			}
			continue
		}

		fileCount++
		totalUncomp += f.UncompressedSize64
		totalComp += f.CompressedSize64

		// Extension histogram.
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != "" {
			extHist[ext]++
		}

		// Compression method tracking.
		methodName := zipMethodName(f.Method)
		comprMethods[methodName]++

		// Top-level entry.
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) == 1 {
			topLevel[parts[0]] = true
		} else if len(parts) > 0 && parts[0] != "" {
			topLevel[parts[0]+"/"] = true
		}

		// Track largest files.
		largest = append(largest, zipFileInfo{
			name: f.Name,
			size: f.UncompressedSize64,
		})

		// Encrypted detection.
		if f.Flags&0x1 != 0 {
			encrypted = true
		}

		// Modification time tracking.
		modTime := f.Modified
		if modTime.IsZero() {
			modTime = f.ModTime()
		}
		if !modTime.IsZero() {
			if !timeSet {
				minTime = modTime
				maxTime = modTime
				timeSet = true
			} else {
				if modTime.Before(minTime) {
					minTime = modTime
				}
				if modTime.After(maxTime) {
					maxTime = modTime
				}
			}
		}

		// JAR MANIFEST.MF parsing.
		if family == "jar" && strings.EqualFold(f.Name, "META-INF/MANIFEST.MF") {
			rc, err := f.Open()
			if err == nil {
				data, err := io.ReadAll(io.LimitReader(rc, 8192))
				rc.Close()
				if err == nil {
					manifestContent = string(data)
				}
			}
		}
	}

	// Sort largest descending, keep top 5.
	sort.Slice(largest, func(i, j int) bool {
		return largest[i].size > largest[j].size
	})
	if len(largest) > 5 {
		largest = largest[:5]
	}

	// Build summary.
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Format: %s\n", family)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Files: %d, Directories: %d\n", fileCount, dirCount)
	fmt.Fprintf(&summary, "Total uncompressed: %s\n", formatSize(totalUncomp))
	if totalUncomp > 0 && totalComp > 0 {
		ratio := float64(totalComp) / float64(totalUncomp) * 100
		fmt.Fprintf(&summary, "Compression ratio: %.1f%%\n", ratio)
	}
	if encrypted {
		summary.WriteString("Encrypted: yes\n")
	}

	// Top-level structure.
	if len(topLevel) > 0 {
		summary.WriteString("\nTop-level structure:\n")
		entries := sortedKeys(topLevel)
		for _, entry := range entries {
			fmt.Fprintf(&summary, "  - %s\n", entry)
		}
	}

	// Extension histogram.
	if len(extHist) > 0 {
		summary.WriteString("\nExtension histogram:\n")
		type extCount struct {
			ext   string
			count int
		}
		var sorted []extCount
		for ext, count := range extHist {
			sorted = append(sorted, extCount{ext, count})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].count != sorted[j].count {
				return sorted[i].count > sorted[j].count
			}
			return sorted[i].ext < sorted[j].ext
		})
		for _, ec := range sorted {
			fmt.Fprintf(&summary, "  - %s: %d\n", ec.ext, ec.count)
		}
	}

	// Largest files.
	if len(largest) > 0 {
		summary.WriteString("\nLargest files:\n")
		for _, f := range largest {
			fmt.Fprintf(&summary, "  - %s (%s)\n", f.name, formatSize(f.size))
		}
	}

	// JAR manifest.
	if manifestContent != "" {
		summary.WriteString("\nMANIFEST.MF:\n")
		lines := strings.Split(manifestContent, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Fprintf(&summary, "  %s\n", line)
			}
		}
	}

	// Enhancement mode extras.
	if e.formatterProfile == OutputProfileEnhancement {
		if timeSet && !minTime.Equal(maxTime) {
			summary.WriteString("\nModification time range:\n")
			fmt.Fprintf(&summary, "  - Earliest: %s\n", minTime.Format(time.RFC3339))
			fmt.Fprintf(&summary, "  - Latest: %s\n", maxTime.Format(time.RFC3339))
		}

		if len(comprMethods) > 0 {
			summary.WriteString("\nCompression methods:\n")
			for method, count := range comprMethods {
				fmt.Fprintf(&summary, "  - %s: %d files\n", method, count)
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreTAR explores an uncompressed tar archive.
func (e *ArchiveExplorer) exploreTAR(input ExploreInput, r io.Reader) (ExploreResult, error) {
	if r == nil {
		r = bytes.NewReader(input.Content)
	}
	return e.exploreTARReader(input, r, "tar")
}

// exploreTARCompressed explores a compressed tar archive.
func (e *ArchiveExplorer) exploreTARCompressed(input ExploreInput, compression string) (ExploreResult, error) {
	r := bytes.NewReader(input.Content)

	var decompressed io.Reader
	var err error

	switch compression {
	case "gzip":
		decompressed, err = gzip.NewReader(r)
		if err != nil {
			return e.compressedFallback(input, "tar.gz", err)
		}
		defer decompressed.(*gzip.Reader).Close()
	case "bzip2":
		decompressed = bzip2.NewReader(r)
	case "zstd":
		dec, err := zstd.NewReader(r)
		if err != nil {
			return e.compressedFallback(input, "tar.zst", err)
		}
		defer dec.Close()
		decompressed = dec
	default:
		return e.compressedFallback(input, compression, fmt.Errorf("unsupported compression: %s", compression))
	}

	format := "tar." + compression
	if compression == "gzip" {
		format = "tar.gz"
	}
	return e.exploreTARReader(input, decompressed, format)
}

// exploreTARReader iterates tar headers and produces a summary.
func (e *ArchiveExplorer) exploreTARReader(input ExploreInput, r io.Reader, format string) (ExploreResult, error) {
	tr := tar.NewReader(r)

	var (
		fileCount    int
		dirCount     int
		symlinkCount int
		totalSize    int64
		extHist      = make(map[string]int)
		topLevel     = make(map[string]bool)
		owners       = make(map[string]int)
		permissions  = make(map[string]int)
		largest      []tarFileInfo
		minTime      time.Time
		maxTime      time.Time
		timeSet      bool
	)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Partial read is acceptable; report what we have.
			break
		}

		// Top-level entry.
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			if hdr.Typeflag == tar.TypeDir && len(parts) == 1 {
				topLevel[parts[0]+"/"] = true
			} else if len(parts) == 1 {
				topLevel[parts[0]] = true
			} else {
				topLevel[parts[0]+"/"] = true
			}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dirCount++
		case tar.TypeSymlink, tar.TypeLink:
			symlinkCount++
		default:
			fileCount++
			totalSize += hdr.Size

			ext := strings.ToLower(filepath.Ext(hdr.Name))
			if ext != "" {
				extHist[ext]++
			}

			largest = append(largest, tarFileInfo{
				name: hdr.Name,
				size: hdr.Size,
			})
		}

		// Permissions tracking.
		perm := hdr.FileInfo().Mode().Perm().String()
		permissions[perm]++

		// Ownership tracking.
		owner := hdr.Uname
		if owner == "" && hdr.Uid >= 0 {
			owner = fmt.Sprintf("uid:%d", hdr.Uid)
		}
		if owner != "" {
			owners[owner]++
		}

		// Modification time.
		if !hdr.ModTime.IsZero() {
			if !timeSet {
				minTime = hdr.ModTime
				maxTime = hdr.ModTime
				timeSet = true
			} else {
				if hdr.ModTime.Before(minTime) {
					minTime = hdr.ModTime
				}
				if hdr.ModTime.After(maxTime) {
					maxTime = hdr.ModTime
				}
			}
		}

		// Discard the entry body to advance the reader.
		if _, err := io.Copy(io.Discard, tr); err != nil {
			break
		}
	}

	// Sort largest descending, keep top 5.
	sort.Slice(largest, func(i, j int) bool {
		return largest[i].size > largest[j].size
	})
	if len(largest) > 5 {
		largest = largest[:5]
	}

	// Build summary.
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Files: %d, Directories: %d", fileCount, dirCount)
	if symlinkCount > 0 {
		fmt.Fprintf(&summary, ", Symlinks: %d", symlinkCount)
	}
	summary.WriteString("\n")
	fmt.Fprintf(&summary, "Total uncompressed: %s\n", formatSize(uint64(totalSize)))

	// Top-level structure.
	if len(topLevel) > 0 {
		summary.WriteString("\nTop-level structure:\n")
		entries := sortedKeys(topLevel)
		for _, entry := range entries {
			fmt.Fprintf(&summary, "  - %s\n", entry)
		}
	}

	// Extension histogram.
	if len(extHist) > 0 {
		summary.WriteString("\nExtension histogram:\n")
		type extCount struct {
			ext   string
			count int
		}
		var sorted []extCount
		for ext, count := range extHist {
			sorted = append(sorted, extCount{ext, count})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].count != sorted[j].count {
				return sorted[i].count > sorted[j].count
			}
			return sorted[i].ext < sorted[j].ext
		})
		for _, ec := range sorted {
			fmt.Fprintf(&summary, "  - %s: %d\n", ec.ext, ec.count)
		}
	}

	// Ownership summary.
	if len(owners) > 0 {
		summary.WriteString("\nOwnership:\n")
		for owner, count := range owners {
			fmt.Fprintf(&summary, "  - %s: %d entries\n", owner, count)
		}
	}

	// Permissions summary.
	if len(permissions) > 0 {
		summary.WriteString("\nPermissions:\n")
		for perm, count := range permissions {
			fmt.Fprintf(&summary, "  - %s: %d entries\n", perm, count)
		}
	}

	// Largest files.
	if len(largest) > 0 {
		summary.WriteString("\nLargest files:\n")
		for _, f := range largest {
			fmt.Fprintf(&summary, "  - %s (%s)\n", f.name, formatSize(uint64(f.size)))
		}
	}

	// Enhancement mode extras.
	if e.formatterProfile == OutputProfileEnhancement {
		if timeSet && !minTime.Equal(maxTime) {
			summary.WriteString("\nModification time range:\n")
			fmt.Fprintf(&summary, "  - Earliest: %s\n", minTime.Format(time.RFC3339))
			fmt.Fprintf(&summary, "  - Latest: %s\n", maxTime.Format(time.RFC3339))
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreGzip handles standalone .gz files. Tries tar first, falls back to
// reporting the gzip container.
func (e *ArchiveExplorer) exploreGzip(input ExploreInput) (ExploreResult, error) {
	gr, err := gzip.NewReader(bytes.NewReader(input.Content))
	if err != nil {
		return e.exploreOpaque(input, "gzip")
	}
	defer gr.Close()

	// Peek to see if it contains a tar archive.
	var buf bytes.Buffer
	_, err = io.Copy(&buf, gr)
	if err != nil {
		return e.exploreOpaque(input, "gzip")
	}

	tarData := buf.Bytes()
	if isTAR(tarData) {
		return e.exploreTARReader(input, bytes.NewReader(tarData), "tar.gz")
	}

	// Plain gzip file.
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	summary.WriteString("Format: gzip\n")
	fmt.Fprintf(&summary, "Compressed size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Uncompressed size: %d bytes\n", len(tarData))
	if len(input.Content) > 0 {
		ratio := float64(len(input.Content)) / float64(len(tarData)) * 100
		fmt.Fprintf(&summary, "Compression ratio: %.1f%%\n", ratio)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreBzip2 handles standalone .bz2 files. Tries tar first, falls back
// to reporting the bzip2 container.
func (e *ArchiveExplorer) exploreBzip2(input ExploreInput) (ExploreResult, error) {
	br := bzip2.NewReader(bytes.NewReader(input.Content))

	var buf bytes.Buffer
	_, err := io.Copy(&buf, br)
	if err != nil {
		return e.exploreOpaque(input, "bzip2")
	}

	tarData := buf.Bytes()
	if isTAR(tarData) {
		return e.exploreTARReader(input, bytes.NewReader(tarData), "tar.bz2")
	}

	// Plain bzip2 file.
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	summary.WriteString("Format: bzip2\n")
	fmt.Fprintf(&summary, "Compressed size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Uncompressed size: %d bytes\n", len(tarData))

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreZstd handles standalone .zst files. Tries tar first, falls back
// to reporting the zstd container.
func (e *ArchiveExplorer) exploreZstd(input ExploreInput) (ExploreResult, error) {
	dec, err := zstd.NewReader(bytes.NewReader(input.Content))
	if err != nil {
		return e.exploreOpaque(input, "zstd")
	}
	defer dec.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, dec)
	if err != nil {
		return e.exploreOpaque(input, "zstd")
	}

	tarData := buf.Bytes()
	if isTAR(tarData) {
		return e.exploreTARReader(input, bytes.NewReader(tarData), "tar.zst")
	}

	// Plain zstd file.
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	summary.WriteString("Format: zstd\n")
	fmt.Fprintf(&summary, "Compressed size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Uncompressed size: %d bytes\n", len(tarData))

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreDeb explores Debian .deb files (ar format).
func (e *ArchiveExplorer) exploreDeb(input ExploreInput) (ExploreResult, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	summary.WriteString("Format: deb (ar archive)\n")
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Parse ar member headers. The ar format is:
	//   "!<arch>\n" (8 bytes global header)
	//   Then for each member:
	//     name[16] + mtime[12] + uid[6] + gid[6] + mode[8] + size[10] + "`\n"[2] = 60 bytes
	//     followed by file data (padded to 2-byte boundary)
	const arHeaderLen = 8
	const memberHeaderLen = 60

	data := input.Content
	if len(data) < arHeaderLen || string(data[:arHeaderLen]) != "!<arch>\n" {
		summary.WriteString("Warning: invalid ar header\n")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "archive",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	summary.WriteString("\nMembers:\n")
	pos := arHeaderLen
	for pos+memberHeaderLen <= len(data) {
		name := strings.TrimRight(string(data[pos:pos+16]), " ")
		// Remove trailing "/" that ar uses.
		name = strings.TrimRight(name, "/")
		sizeStr := strings.TrimSpace(string(data[pos+48 : pos+58]))

		var size int
		fmt.Sscanf(sizeStr, "%d", &size)

		fmt.Fprintf(&summary, "  - %s (%s)\n", name, formatSize(uint64(size)))

		// Advance past header + data (2-byte aligned).
		pos += memberHeaderLen + size
		if pos%2 != 0 {
			pos++
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreRPM explores RPM package files by parsing the 96-byte lead.
func (e *ArchiveExplorer) exploreRPM(input ExploreInput) (ExploreResult, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	summary.WriteString("Format: RPM package\n")
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	data := input.Content
	// RPM lead is 96 bytes.
	const rpmLeadLen = 96
	if len(data) < rpmLeadLen {
		summary.WriteString("Warning: file too small for RPM lead\n")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "archive",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	// Verify magic: 0xedabeedb.
	if data[0] != 0xed || data[1] != 0xab || data[2] != 0xee || data[3] != 0xdb {
		summary.WriteString("Warning: invalid RPM magic\n")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "archive",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	// RPM version: major at offset 4, minor at offset 5.
	major := data[4]
	minor := data[5]
	fmt.Fprintf(&summary, "RPM version: %d.%d\n", major, minor)

	// Type at offset 6-7 (big-endian uint16): 0=binary, 1=source.
	rpmType := uint16(data[6])<<8 | uint16(data[7])
	switch rpmType {
	case 0:
		summary.WriteString("Type: binary\n")
	case 1:
		summary.WriteString("Type: source\n")
	default:
		fmt.Fprintf(&summary, "Type: unknown (%d)\n", rpmType)
	}

	// Name at offset 10, up to 66 bytes (null-terminated).
	nameBytes := data[10:76]
	nameEnd := bytes.IndexByte(nameBytes, 0)
	if nameEnd < 0 {
		nameEnd = len(nameBytes)
	}
	name := string(nameBytes[:nameEnd])
	if name != "" {
		fmt.Fprintf(&summary, "Package name: %s\n", name)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreOpaque handles formats we can only identify but not list.
func (e *ArchiveExplorer) exploreOpaque(input ExploreInput, family string) (ExploreResult, error) {
	displayName := family
	if displayName == "" {
		displayName = "unknown archive"
	}

	// Try magic byte identification for better naming.
	if family == "" {
		for _, sig := range magicSignatures {
			end := sig.offset + len(sig.magic)
			if len(input.Content) >= end &&
				bytes.Equal(input.Content[sig.offset:end], sig.magic) {
				displayName = sig.name
				break
			}
		}
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Format: %s\n", displayName)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	summary.WriteString("Note: contents cannot be listed without external tools\n")

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// compressedFallback returns an error summary for a compressed archive we
// cannot decompress.
func (e *ArchiveExplorer) compressedFallback(input ExploreInput, format string, err error) (ExploreResult, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "Archive file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	fmt.Fprintf(&summary, "Error: could not decompress: %v\n", err)

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "archive",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// isTAR checks whether data looks like a tar archive by checking the ustar
// magic at offset 257.
func isTAR(data []byte) bool {
	if len(data) < 262 {
		return false
	}
	return string(data[257:262]) == "ustar"
}

// zipMethodName returns a human-readable name for a ZIP compression method.
func zipMethodName(method uint16) string {
	switch method {
	case zip.Store:
		return "Store"
	case zip.Deflate:
		return "Deflate"
	default:
		return fmt.Sprintf("Method(%d)", method)
	}
}

// formatSize formats a byte count into a human-readable string.
func formatSize(bytes uint64) string {
	const (
		_          = iota
		kb float64 = 1 << (10 * iota)
		mb
		gb
	)
	b := float64(bytes)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", b/gb)
	case b >= mb:
		return fmt.Sprintf("%.1f MB", b/mb)
	case b >= kb:
		return fmt.Sprintf("%.1f KB", b/kb)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// zipFileInfo holds name and size for sorting.
type zipFileInfo struct {
	name string
	size uint64
}

// tarFileInfo holds name and size for sorting.
type tarFileInfo struct {
	name string
	size int64
}
