package explorer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExecutableExplorer explores executable and compiled binary formats.
// It detects ELF, PE/COFF, Mach-O, WASM, Java class, and Python bytecode
// files via both extension and magic byte matching. When available, it shells
// out to platform tools (readelf, otool, objdump, nm, strings, file) to
// extract dependencies, sections, symbols, and interesting strings.
type ExecutableExplorer struct {
	formatterProfile OutputProfile
}

// executableExtensions maps extensions to format family identifiers.
// These 17 extensions are claimed by ExecutableExplorer. Notably .deb, .rpm,
// .dmg, and .jar are NOT claimed here -- they go to ArchiveExplorer.
var executableExtensions = map[string]string{
	"exe":   "pe",
	"dll":   "pe",
	"so":    "elf",
	"dylib": "macho",
	"a":     "static",
	"lib":   "static",
	"o":     "object",
	"obj":   "object",
	"ko":    "elf",
	"sys":   "pe",
	"com":   "dos",
	"bin":   "raw",
	"elf":   "elf",
	"wasm":  "wasm",
	"class": "java",
	"pyc":   "pyc",
	"pyo":   "pyc",
}

// executableMagic describes a magic byte signature for executable formats.
type executableMagic struct {
	name   string
	magic  []byte
	offset int
}

// executableMagicSignatures lists magic bytes for executable formats.
// Order matters: ELF, PE/MZ, Mach-O 32, Mach-O 64, WASM are checked first.
// 0xCAFEBABE is intentionally last because it collides between Mach-O
// Universal (fat binary) and Java .class files.
var executableMagicSignatures = []executableMagic{
	{name: "ELF", magic: []byte{0x7F, 0x45, 0x4C, 0x46}, offset: 0},
	{name: "PE/MZ", magic: []byte{0x4D, 0x5A}, offset: 0},
	{name: "Mach-O 32", magic: []byte{0xFE, 0xED, 0xFA, 0xCE}, offset: 0},
	{name: "Mach-O 64", magic: []byte{0xFE, 0xED, 0xFA, 0xCF}, offset: 0},
	{name: "WASM", magic: []byte{0x00, 0x61, 0x73, 0x6D}, offset: 0},
	// 0xCAFEBABE: Both Mach-O Universal and Java class files share this
	// 4-byte magic. Disambiguation is handled in disambiguateCafebabe()
	// by examining bytes 4-7:
	//   - Java class files have a minor version (u16 BE at 4-5) and major
	//     version (u16 BE at 6-7). Known major versions are 45-67 (Java 1.1
	//     through Java 23). The 32-bit big-endian value at offset 4 will
	//     typically be >= 45 (0x0000002D as a u32).
	//   - Mach-O Universal (fat) binaries have a big-endian u32 architecture
	//     count at offset 4. Real fat binaries have 1-4 architectures, so
	//     this value is < 20.
	// Extension-first dispatch resolves .class files before magic checks.
	{name: "CAFEBABE", magic: []byte{0xCA, 0xFE, 0xBA, 0xBE}, offset: 0},
}

// Tool invocation limits.
const (
	execToolTimeout       = 5 * time.Second
	maxDeps               = 20
	maxSections           = 20
	maxExportedSymbols    = 50
	maxImportedSymbols    = 50
	maxStrings            = 30
	enhancedSymbols       = 100
	enhancedStrings       = 50
	maxStringLineLen      = 160
	interestingStringsMin = 6
)

// CanHandle returns true if the file is an executable or compiled binary.
// Extension-first dispatch means .class files are always recognized even
// though their 0xCAFEBABE magic collides with Mach-O Universal.
func (e *ExecutableExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := executableExtensions[ext]; ok {
		return true
	}
	return e.detectFormatFromMagic(content) != ""
}

// Explore returns a structured summary of the executable file.
func (e *ExecutableExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	family := executableExtensions[ext]
	format := e.resolveFormat(ext, family, input.Content)

	var summary strings.Builder
	fmt.Fprintf(&summary, "Executable file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Format: %s\n", format)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Write content to temp file for tool invocation.
	err := withTempFile("crush-exec-*", input.Content, func(tempPath string) error {
		return e.exploreWithTools(ctx, &summary, tempPath, family, input.Content)
	})
	if err != nil {
		// Non-fatal: we already have basic header info.
		fmt.Fprintf(&summary, "\nNote: tool analysis unavailable: %v\n", err)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "executable",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// resolveFormat returns a human-readable format string from extension family
// and magic byte detection.
func (e *ExecutableExplorer) resolveFormat(ext, family string, content []byte) string {
	switch family {
	case "elf":
		if ext != "" {
			return fmt.Sprintf("ELF (%s)", ext)
		}
		return "ELF"
	case "pe":
		if ext != "" {
			return fmt.Sprintf("PE/COFF (%s)", ext)
		}
		return "PE/COFF"
	case "macho":
		return "Mach-O dynamic library"
	case "static":
		if ext != "" {
			return fmt.Sprintf("Static library (%s)", ext)
		}
		return "Static library"
	case "object":
		if ext != "" {
			return fmt.Sprintf("Object file (%s)", ext)
		}
		return "Object file"
	case "dos":
		return "DOS COM"
	case "raw":
		// Try magic detection for raw .bin files.
		if magic := e.detectFormatFromMagic(content); magic != "" {
			return magic
		}
		return "Raw binary"
	case "wasm":
		return "WebAssembly"
	case "java":
		return "Java class"
	case "pyc":
		return "Python bytecode"
	case "":
		// No extension match. Use magic bytes.
		if magic := e.detectFormatFromMagic(content); magic != "" {
			return magic
		}
		return "Unknown executable"
	default:
		return family
	}
}

// detectFormatFromMagic returns a format name from magic bytes, or empty
// string if no match. Handles 0xCAFEBABE disambiguation.
func (e *ExecutableExplorer) detectFormatFromMagic(content []byte) string {
	for _, sig := range executableMagicSignatures {
		end := sig.offset + len(sig.magic)
		if len(content) < end {
			continue
		}
		if !bytes.Equal(content[sig.offset:end], sig.magic) {
			continue
		}
		switch sig.name {
		case "ELF":
			return "ELF"
		case "PE/MZ":
			return "PE/COFF"
		case "Mach-O 32":
			return "Mach-O 32-bit"
		case "Mach-O 64":
			return "Mach-O 64-bit"
		case "WASM":
			return "WebAssembly"
		case "CAFEBABE":
			return disambiguateCafebabe(content)
		}
	}
	return ""
}

// disambiguateCafebabe resolves the 0xCAFEBABE magic collision between
// Mach-O Universal (fat) binaries and Java .class files.
//
// Bytes 4-7 interpretation:
//   - Mach-O Universal: big-endian u32 architecture count, typically 1-4.
//   - Java class: minor_version (u16 BE at 4-5) + major_version (u16 BE at 6-7).
//     Known major versions: 45 (Java 1.1) through ~67 (Java 23).
//     When read as a single u32 BE, version numbers >= 45 are common.
//
// Heuristic: if the u32 at offset 4 is < 20, it's a fat binary arch count.
// If >= 45, it's a Java class version. Values 20-44 are ambiguous but
// extremely unlikely in practice.
func disambiguateCafebabe(content []byte) string {
	if len(content) < 8 {
		return "Mach-O Universal or Java class (ambiguous)"
	}

	val := binary.BigEndian.Uint32(content[4:8])

	// Mach-O Universal: architecture count is small (typically 1-4, max ~10).
	if val < 20 {
		return "Mach-O Universal"
	}

	// Java class: major version >= 45.
	if val >= 45 {
		return "Java class"
	}

	// Values 20-44: extremely unlikely but technically ambiguous.
	return "Mach-O Universal or Java class (ambiguous)"
}

// exploreWithTools runs external analysis tools against the temp file.
// Each tool is independently optional; all failures are silently ignored.
func (e *ExecutableExplorer) exploreWithTools(
	ctx context.Context, summary *strings.Builder, tempPath, family string, content []byte,
) error {
	// Determine format from magic bytes for tool selection.
	formatHint := e.detectBinaryType(content)

	// Step 1: file -b for format detection.
	if fileDesc := runTool(ctx, "file", "-b", tempPath); fileDesc != "" {
		trimmed := strings.TrimSpace(fileDesc)
		if trimmed != "" && trimmed != "data" {
			fmt.Fprintf(summary, "\nFile type: %s\n", trimmed)
		}
	}

	// Step 2: Dependencies.
	deps := e.extractDependencies(ctx, tempPath, formatHint)
	if len(deps) > 0 {
		summary.WriteString("\nDependencies:\n")
		limit := maxDeps
		for i, dep := range deps {
			if i >= limit {
				fmt.Fprintf(summary, "  - ... and %d more\n", len(deps)-limit)
				break
			}
			fmt.Fprintf(summary, "  - %s\n", dep)
		}
	}

	// Step 3: Sections.
	sections := e.extractSections(ctx, tempPath, formatHint)
	if len(sections) > 0 {
		summary.WriteString("\nSections:\n")
		limit := maxSections
		if e.formatterProfile == OutputProfileEnhancement {
			limit = len(sections) // No limit in enhancement mode.
		}
		for i, sec := range sections {
			if i >= limit {
				fmt.Fprintf(summary, "  - ... and %d more\n", len(sections)-limit)
				break
			}
			fmt.Fprintf(summary, "  - %s\n", sec)
		}
	}

	// Step 4: Symbols via nm -g.
	exportLimit := maxExportedSymbols
	importLimit := maxImportedSymbols
	if e.formatterProfile == OutputProfileEnhancement {
		exportLimit = enhancedSymbols
		importLimit = enhancedSymbols
	}

	exported, imported := e.extractSymbols(ctx, tempPath)
	if len(exported) > 0 {
		summary.WriteString("\nExported symbols:\n")
		for i, sym := range exported {
			if i >= exportLimit {
				fmt.Fprintf(summary, "  - ... and %d more\n", len(exported)-exportLimit)
				break
			}
			fmt.Fprintf(summary, "  - %s\n", sym)
		}
	}
	if len(imported) > 0 {
		summary.WriteString("\nImported symbols:\n")
		for i, sym := range imported {
			if i >= importLimit {
				fmt.Fprintf(summary, "  - ... and %d more\n", len(imported)-importLimit)
				break
			}
			fmt.Fprintf(summary, "  - %s\n", sym)
		}
	}

	// Step 5: Interesting strings.
	strLimit := maxStrings
	if e.formatterProfile == OutputProfileEnhancement {
		strLimit = enhancedStrings
	}
	interesting := e.extractStrings(ctx, tempPath, strLimit)
	if len(interesting) > 0 {
		summary.WriteString("\nInteresting strings:\n")
		for _, s := range interesting {
			fmt.Fprintf(summary, "  - %s\n", s)
		}
	}

	return nil
}

// detectBinaryType returns "elf", "pe", "macho", "wasm", "java", or ""
// based on magic bytes, for tool selection.
func (e *ExecutableExplorer) detectBinaryType(content []byte) string {
	if len(content) >= 4 {
		if bytes.Equal(content[:4], []byte{0x7F, 0x45, 0x4C, 0x46}) {
			return "elf"
		}
		if bytes.Equal(content[:2], []byte{0x4D, 0x5A}) {
			return "pe"
		}
		if bytes.Equal(content[:4], []byte{0xFE, 0xED, 0xFA, 0xCE}) ||
			bytes.Equal(content[:4], []byte{0xFE, 0xED, 0xFA, 0xCF}) ||
			bytes.Equal(content[:4], []byte{0xCA, 0xFE, 0xBA, 0xBE}) {
			return "macho"
		}
		if bytes.Equal(content[:4], []byte{0x00, 0x61, 0x73, 0x6D}) {
			return "wasm"
		}
	}
	return ""
}

// extractDependencies extracts shared library dependencies using
// format-appropriate tools.
func (e *ExecutableExplorer) extractDependencies(ctx context.Context, tempPath, formatHint string) []string {
	switch formatHint {
	case "elf":
		// Try readelf -d first.
		if output := runTool(ctx, "readelf", "-d", tempPath); output != "" {
			deps := parseELFDeps(output)
			if len(deps) > 0 {
				return deps
			}
		}
		// Fallback to ldd (less safe, but commonly available).
		if output := runTool(ctx, "ldd", tempPath); output != "" {
			return parseLddDeps(output)
		}
	case "macho":
		if output := runTool(ctx, "otool", "-L", tempPath); output != "" {
			return parseMachODeps(output)
		}
	case "pe":
		if output := runTool(ctx, "objdump", "-p", tempPath); output != "" {
			return parsePEDeps(output)
		}
	}
	return nil
}

// extractSections extracts section information using format-appropriate tools.
func (e *ExecutableExplorer) extractSections(ctx context.Context, tempPath, formatHint string) []string {
	switch formatHint {
	case "elf":
		if output := runTool(ctx, "readelf", "-S", tempPath); output != "" {
			return parseELFSections(output)
		}
	case "macho":
		if output := runTool(ctx, "otool", "-l", tempPath); output != "" {
			return parseMachOSections(output)
		}
	}
	return nil
}

// extractSymbols extracts exported and imported symbols via nm -g.
func (e *ExecutableExplorer) extractSymbols(ctx context.Context, tempPath string) (exported, imported []string) {
	output := runTool(ctx, "nm", "-g", tempPath)
	if output == "" {
		return nil, nil
	}
	return parseNmSymbols(output)
}

// extractStrings runs `strings -n 6` and filters for interesting patterns.
func (e *ExecutableExplorer) extractStrings(ctx context.Context, tempPath string, limit int) []string {
	output := runTool(ctx, "strings", fmt.Sprintf("-n%d", interestingStringsMin), tempPath)
	if output == "" {
		return nil
	}
	return filterInterestingStrings(output, limit)
}

// runTool runs an external tool with a 5-second timeout. Returns empty string
// if the tool is not found or fails.
func runTool(ctx context.Context, name string, args ...string) string {
	// Check tool availability before invocation.
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}

	timeout, cancel := context.WithTimeout(ctx, execToolTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeout, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// --- Output parsers ---

// parseELFDeps extracts NEEDED shared library names from readelf -d output.
func parseELFDeps(output string) []string {
	var deps []string
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "NEEDED") {
			continue
		}
		// Pattern: ... Shared library: [libfoo.so.6]
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start >= 0 && end > start {
			deps = append(deps, line[start+1:end])
		}
		if len(deps) >= maxDeps {
			break
		}
	}
	return deps
}

// parseLddDeps extracts library names from ldd output.
func parseLddDeps(output string) []string {
	var deps []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip linux-vdso and similar virtual DSOs.
		if strings.Contains(line, "linux-vdso") || strings.Contains(line, "linux-gate") {
			continue
		}
		// Pattern: libfoo.so.6 => /path/to/libfoo.so.6 (0x...)
		// or: /lib64/ld-linux-x86-64.so.2 (0x...)
		parts := strings.Fields(line)
		if len(parts) > 0 {
			deps = append(deps, parts[0])
		}
		if len(deps) >= maxDeps {
			break
		}
	}
	return deps
}

// parseMachODeps extracts dependency paths from otool -L output.
func parseMachODeps(output string) []string {
	var deps []string
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		// First line is the binary path itself; skip it.
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Pattern: /usr/lib/libSystem.B.dylib (compatibility version ...)
		if idx := strings.Index(line, " (compatibility"); idx > 0 {
			deps = append(deps, line[:idx])
		} else if idx := strings.Index(line, " ("); idx > 0 {
			deps = append(deps, line[:idx])
		} else {
			deps = append(deps, line)
		}
		if len(deps) >= maxDeps {
			break
		}
	}
	return deps
}

// parsePEDeps extracts DLL names from objdump -p output.
func parsePEDeps(output string) []string {
	var deps []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DLL Name:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "DLL Name:"))
			if name != "" {
				deps = append(deps, name)
			}
		}
		if len(deps) >= maxDeps {
			break
		}
	}
	return deps
}

// parseNmSymbols parses nm -g output into exported and imported symbol lists.
// Exported symbols have types T, D, B, R, etc. (uppercase).
// Imported/undefined symbols have type U.
func parseNmSymbols(output string) (exported, imported []string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		var symType, symName string
		if len(fields) == 2 {
			// Pattern: "U symbol_name" (undefined).
			symType = fields[0]
			symName = fields[1]
		} else {
			// Pattern: "0000000000001234 T symbol_name".
			symType = fields[1]
			symName = fields[2]
		}

		if symType == "U" || symType == "u" {
			imported = append(imported, symName)
		} else if symType >= "A" && symType <= "Z" {
			exported = append(exported, symName)
		}
	}
	return exported, imported
}

// parseELFSections extracts section names and types from readelf -S output.
var elfSectionRe = regexp.MustCompile(`\[\s*\d+\]\s+(\S+)\s+(\S+)`)

func parseELFSections(output string) []string {
	var sections []string
	for _, line := range strings.Split(output, "\n") {
		matches := elfSectionRe.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		name := matches[1]
		typ := matches[2]
		if name == "" || typ == "NULL" {
			continue
		}
		sections = append(sections, fmt.Sprintf("%s (%s)", name, typ))
	}
	return sections
}

// parseMachOSections extracts section names from otool -l output.
func parseMachOSections(output string) []string {
	var sections []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "sectname ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "sectname"))
			if name != "" {
				sections = append(sections, name)
			}
		}
	}
	return sections
}

// interestingStringPatterns matches URLs, paths, errors, versions, warnings.
var interestingStringPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://`),                             // URLs.
	regexp.MustCompile(`^/[a-z]`),                                    // Unix paths.
	regexp.MustCompile(`(?i)^(error|fatal|warning|warn|panic|fail)`), // Error messages.
	regexp.MustCompile(`(?i)^v?\d+\.\d+\.\d+`),                       // Version strings.
	regexp.MustCompile(`(?i)(version|copyright|license|author)`),     // Metadata.
}

// filterInterestingStrings filters `strings` output for URLs, paths, errors,
// and version strings, returning at most limit entries.
func filterInterestingStrings(output string, limit int) []string {
	var result []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) > maxStringLineLen {
			continue
		}
		for _, pat := range interestingStringPatterns {
			if pat.MatchString(line) {
				result = append(result, line)
				break
			}
		}
		if len(result) >= limit {
			break
		}
	}
	return result
}
