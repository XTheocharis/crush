package explorer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- Synthetic header builders ---

// buildSyntheticELF creates a minimal 64-bit ELF header (52 bytes minimum
// for a valid ELF64 header, padded to 64 for alignment).
//
// Layout:
//
//	Offset 0-3:   Magic: 0x7f 0x45 0x4c 0x46 (\x7fELF)
//	Offset 4:     Class: 2 (64-bit)
//	Offset 5:     Data: 1 (little-endian)
//	Offset 6:     Version: 1 (EV_CURRENT)
//	Offset 16-17: Type: 2 (ET_EXEC, little-endian uint16)
//	Offset 18-19: Machine: 0x3e (x86_64, little-endian uint16)
func buildSyntheticELF(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 64)
	// Magic: \x7fELF
	data[0] = 0x7F
	data[1] = 'E'
	data[2] = 'L'
	data[3] = 'F'
	// Class: ELFCLASS64.
	data[4] = 2
	// Data: ELFDATA2LSB (little-endian).
	data[5] = 1
	// Version: EV_CURRENT.
	data[6] = 1
	// OS/ABI: ELFOSABI_NONE.
	data[7] = 0
	// Type: ET_EXEC (offset 16, little-endian uint16).
	data[16] = 2
	data[17] = 0
	// Machine: x86_64 (0x3E, little-endian uint16).
	data[18] = 0x3E
	data[19] = 0
	return data
}

// buildSyntheticPE creates a minimal PE/MZ header for testing.
// DOS header with "MZ" at offset 0, e_lfanew at offset 0x3C pointing to
// PE signature "PE\0\0" at offset 64.
func buildSyntheticPE(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 128)
	data[0] = 'M'
	data[1] = 'Z'
	// e_lfanew at offset 0x3C (60): point to PE signature at offset 64.
	binary.LittleEndian.PutUint32(data[0x3C:], 64)
	// PE signature "PE\0\0" at offset 64.
	data[64] = 'P'
	data[65] = 'E'
	data[66] = 0
	data[67] = 0
	return data
}

// buildSyntheticMachO64 creates a minimal Mach-O 64-bit header.
func buildSyntheticMachO64(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 32)
	// Magic: 0xFEEDFACF (Mach-O 64-bit).
	data[0] = 0xFE
	data[1] = 0xED
	data[2] = 0xFA
	data[3] = 0xCF
	return data
}

// buildSyntheticMachO32 creates a minimal Mach-O 32-bit header.
func buildSyntheticMachO32(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 32)
	// Magic: 0xFEEDFACE (Mach-O 32-bit).
	data[0] = 0xFE
	data[1] = 0xED
	data[2] = 0xFA
	data[3] = 0xCE
	return data
}

// buildSyntheticMachOUniversal creates a minimal Mach-O Universal (fat)
// header. The fat header count at offset 4-7 is set to 2 architectures.
func buildSyntheticMachOUniversal(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 32)
	// Magic: 0xCAFEBABE.
	data[0] = 0xCA
	data[1] = 0xFE
	data[2] = 0xBA
	data[3] = 0xBE
	// Fat header count: 2 (big-endian uint32).
	binary.BigEndian.PutUint32(data[4:8], 2)
	return data
}

// buildSyntheticJavaClass creates a minimal Java .class header.
// Major version 52 = Java 8.
func buildSyntheticJavaClass(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 16)
	// Magic: 0xCAFEBABE.
	data[0] = 0xCA
	data[1] = 0xFE
	data[2] = 0xBA
	data[3] = 0xBE
	// Minor version (offset 4-5): 0.
	data[4] = 0
	data[5] = 0
	// Major version (offset 6-7): 52 (big-endian) = Java 8.
	binary.BigEndian.PutUint16(data[6:8], 52)
	return data
}

// buildSyntheticWASM creates a minimal WASM header.
func buildSyntheticWASM(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 8)
	// Magic: \x00asm
	data[0] = 0x00
	data[1] = 'a'
	data[2] = 's'
	data[3] = 'm'
	// Version 1.
	data[4] = 1
	data[5] = 0
	data[6] = 0
	data[7] = 0
	return data
}

// --- CanHandle tests ---

func TestExecutableExplorer_CanHandle_Extensions(t *testing.T) {
	t.Parallel()

	explorer := &ExecutableExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		// All 17 executable extensions.
		{name: "exe", path: "program.exe", expected: true},
		{name: "dll", path: "library.dll", expected: true},
		{name: "so", path: "libfoo.so", expected: true},
		{name: "dylib", path: "libbar.dylib", expected: true},
		{name: "a", path: "libstatic.a", expected: true},
		{name: "lib", path: "foo.lib", expected: true},
		{name: "o", path: "main.o", expected: true},
		{name: "obj", path: "main.obj", expected: true},
		{name: "ko", path: "driver.ko", expected: true},
		{name: "sys", path: "driver.sys", expected: true},
		{name: "elf", path: "program.elf", expected: true},
		{name: "bin", path: "firmware.bin", expected: true},
		{name: "com", path: "command.com", expected: true},
		{name: "wasm", path: "module.wasm", expected: true},
		{name: "class", path: "Main.class", expected: true},
		{name: "pyc", path: "module.pyc", expected: true},
		{name: "pyo", path: "module.pyo", expected: true},

		// Case insensitivity.
		{name: "EXE uppercase", path: "PROGRAM.EXE", expected: true},
		{name: "DLL uppercase", path: "LIBRARY.DLL", expected: true},

		// NOT claimed: these go to ArchiveExplorer.
		{name: "deb not executable", path: "package.deb", expected: false},
		{name: "rpm not executable", path: "package.rpm", expected: false},
		{name: "dmg not executable", path: "image.dmg", expected: false},
		{name: "jar not executable", path: "app.jar", expected: false},

		// Non-executable extensions.
		{name: "json", path: "data.json", expected: false},
		{name: "go", path: "main.go", expected: false},
		{name: "txt", path: "readme.txt", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content := tt.content
			if content == nil {
				content = []byte("dummy content")
			}
			result := explorer.CanHandle(tt.path, content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExecutableExplorer_CanHandle_MagicBytes(t *testing.T) {
	t.Parallel()

	explorer := &ExecutableExplorer{}

	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "ELF magic",
			content:  buildSyntheticELF(t),
			expected: true,
		},
		{
			name:     "PE/MZ magic",
			content:  buildSyntheticPE(t),
			expected: true,
		},
		{
			name:     "Mach-O 32-bit magic",
			content:  buildSyntheticMachO32(t),
			expected: true,
		},
		{
			name:     "Mach-O 64-bit magic",
			content:  buildSyntheticMachO64(t),
			expected: true,
		},
		{
			name:     "Mach-O Universal magic",
			content:  buildSyntheticMachOUniversal(t),
			expected: true,
		},
		{
			name:     "WASM magic",
			content:  buildSyntheticWASM(t),
			expected: true,
		},
		{
			name:     "Java class magic",
			content:  buildSyntheticJavaClass(t),
			expected: true,
		},
		{
			name:     "no magic",
			content:  bytes.Repeat([]byte{0x42}, 64),
			expected: false,
		},
		{
			name:     "too short",
			content:  []byte{0x7F, 'E'},
			expected: false,
		},
		{
			name:     "empty",
			content:  []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Use extensionless path to force magic byte detection.
			result := explorer.CanHandle("unknownfile", tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestExecutableExplorer_CafebebeCollision verifies that the 0xCAFEBABE
// collision is handled correctly: .class extension -> Java, extensionless
// with fat header count < 20 -> Mach-O Universal, extensionless with major
// version >= 45 -> Java class.
func TestExecutableExplorer_CafebebeCollision(t *testing.T) {
	t.Parallel()

	explorer := &ExecutableExplorer{}

	t.Run("class extension always Java", func(t *testing.T) {
		t.Parallel()
		// Even with a Mach-O Universal header (fat count = 2), the .class
		// extension should resolve to Java.
		content := buildSyntheticMachOUniversal(t)
		require.True(t, explorer.CanHandle("Main.class", content))

		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "Main.class",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "executable", result.ExplorerUsed)
		require.Contains(t, result.Summary, "Java class")
	})

	t.Run("extensionless fat binary", func(t *testing.T) {
		t.Parallel()
		content := buildSyntheticMachOUniversal(t)
		e := &ExecutableExplorer{}
		format := e.detectFormatFromMagic(content)
		require.Equal(t, "Mach-O Universal", format,
			"fat header count 2 should disambiguate as Mach-O Universal")
	})

	t.Run("extensionless Java class", func(t *testing.T) {
		t.Parallel()
		content := buildSyntheticJavaClass(t)
		e := &ExecutableExplorer{}
		format := e.detectFormatFromMagic(content)
		require.Equal(t, "Java class", format,
			"major version 52 should disambiguate as Java class")
	})

	t.Run("ambiguous short content", func(t *testing.T) {
		t.Parallel()
		// Only 4 bytes = can't disambiguate.
		content := []byte{0xCA, 0xFE, 0xBA, 0xBE}
		e := &ExecutableExplorer{}
		format := e.detectFormatFromMagic(content)
		require.Contains(t, format, "ambiguous")
	})
}

// --- Explore tests ---

func TestExecutableExplorer_Explore_SyntheticELF(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "program.elf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)

	s := result.Summary
	require.Contains(t, s, "Executable file: program.elf")
	require.Contains(t, s, "Format: ELF (elf)")
	require.Contains(t, s, "Size: 64 bytes")
}

func TestExecutableExplorer_Explore_SyntheticPE(t *testing.T) {
	t.Parallel()

	content := buildSyntheticPE(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "program.exe",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)

	s := result.Summary
	require.Contains(t, s, "Executable file: program.exe")
	require.Contains(t, s, "Format: PE/COFF (exe)")
	require.Contains(t, s, "Size: 128 bytes")
}

func TestExecutableExplorer_Explore_SyntheticMachO(t *testing.T) {
	t.Parallel()

	content := buildSyntheticMachO64(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "program.dylib",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: program.dylib")
	require.Contains(t, s, "Format: Mach-O dynamic library")
}

func TestExecutableExplorer_Explore_SyntheticWASM(t *testing.T) {
	t.Parallel()

	content := buildSyntheticWASM(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "module.wasm",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: module.wasm")
	require.Contains(t, s, "Format: WebAssembly")
}

func TestExecutableExplorer_Explore_JavaClass(t *testing.T) {
	t.Parallel()

	content := buildSyntheticJavaClass(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "Main.class",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: Main.class")
	require.Contains(t, s, "Format: Java class")
}

func TestExecutableExplorer_Explore_PythonBytecode(t *testing.T) {
	t.Parallel()

	// Python .pyc file: magic number varies by version but we detect by
	// extension so content doesn't matter for CanHandle.
	content := make([]byte, 32)
	content[0] = 0x42
	content[1] = 0x0D

	explorer := &ExecutableExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "module.pyc",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: module.pyc")
	require.Contains(t, s, "Format: Python bytecode")
}

func TestExecutableExplorer_Explore_ExtensionlessELF(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "myprogram",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: myprogram")
	require.Contains(t, s, "Format: ELF")
}

func TestExecutableExplorer_Explore_ObjectFile(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "main.o",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Format: Object file (o)")
}

func TestExecutableExplorer_Explore_StaticLibrary(t *testing.T) {
	t.Parallel()

	content := []byte("!<arch>\n") // ar archive header, same as .a files.
	content = append(content, bytes.Repeat([]byte{0}, 64)...)

	explorer := &ExecutableExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "libfoo.a",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Format: Static library (a)")
}

// --- Enhancement mode tests ---

func TestExecutableExplorer_EnhancementMode(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)

	t.Run("enhancement profile set", func(t *testing.T) {
		t.Parallel()
		explorer := &ExecutableExplorer{formatterProfile: OutputProfileEnhancement}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "program.elf",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "executable", result.ExplorerUsed)
	})

	t.Run("parity profile set", func(t *testing.T) {
		t.Parallel()
		explorer := &ExecutableExplorer{formatterProfile: OutputProfileParity}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "program.elf",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "executable", result.ExplorerUsed)
	})
}

// --- Parser unit tests ---

func TestParseELFDeps(t *testing.T) {
	t.Parallel()

	output := ` 0x0000000000000001 (NEEDED)             Shared library: [libm.so.6]
 0x0000000000000001 (NEEDED)             Shared library: [libc.so.6]
 0x000000000000000e (SONAME)             Library soname: [libfoo.so.1]
`
	deps := parseELFDeps(output)
	require.Equal(t, []string{"libm.so.6", "libc.so.6"}, deps)
}

func TestParseLddDeps(t *testing.T) {
	t.Parallel()

	output := "	linux-vdso.so.1 (0x00007ffff7fc1000)\n" +
		"	libm.so.6 => /lib/x86_64-linux-gnu/libm.so.6 (0x00007ffff7e00000)\n" +
		"	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007ffff7c00000)\n" +
		"	/lib64/ld-linux-x86-64.so.2 (0x00007ffff7fc3000)\n"

	deps := parseLddDeps(output)
	// linux-vdso is filtered out.
	require.Contains(t, deps, "libm.so.6")
	require.Contains(t, deps, "libc.so.6")
}

func TestParseMachODeps(t *testing.T) {
	t.Parallel()

	output := `/usr/local/bin/myprogram:
	/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1292.0.0)
	/usr/lib/libc++.1.dylib (compatibility version 1.0.0, current version 902.1.0)
`
	deps := parseMachODeps(output)
	require.Equal(t, []string{
		"/usr/lib/libSystem.B.dylib",
		"/usr/lib/libc++.1.dylib",
	}, deps)
}

func TestParsePEDeps(t *testing.T) {
	t.Parallel()

	output := `
The Import Tables (Hint-Name Table):
	DLL Name: KERNEL32.dll
	Hint/Ord  Name
	    123  CreateFileW
	DLL Name: USER32.dll
	Hint/Ord  Name
	    456  MessageBoxW
`
	deps := parsePEDeps(output)
	require.Equal(t, []string{"KERNEL32.dll", "USER32.dll"}, deps)
}

func TestParseNmSymbols(t *testing.T) {
	t.Parallel()

	output := `0000000000001234 T main
0000000000005678 T helper_func
                 U printf
                 U malloc
0000000000009abc D global_var
`
	exported, imported := parseNmSymbols(output)
	require.Contains(t, exported, "main")
	require.Contains(t, exported, "helper_func")
	require.Contains(t, exported, "global_var")
	require.Contains(t, imported, "printf")
	require.Contains(t, imported, "malloc")
}

func TestParseELFSections(t *testing.T) {
	t.Parallel()

	output := `There are 14 section headers, starting at offset 0x1234:

Section Headers:
  [Nr] Name              Type             Address           Offset
       Size              EntSize          Flags  Link  Info  Align
  [ 0]                   NULL             0000000000000000  00000000
       0000000000000000  0000000000000000           0     0     0
  [ 1] .text             PROGBITS         0000000000001000  00001000
       0000000000001234  0000000000000000  AX       0     0     16
  [ 2] .data             PROGBITS         0000000000003000  00003000
       0000000000000100  0000000000000000  WA       0     0     8
  [ 3] .bss              NOBITS           0000000000004000  00004000
       0000000000000200  0000000000000000  WA       0     0     16
`
	sections := parseELFSections(output)
	require.Contains(t, sections, ".text (PROGBITS)")
	require.Contains(t, sections, ".data (PROGBITS)")
	require.Contains(t, sections, ".bss (NOBITS)")
}

func TestParseMachOSections(t *testing.T) {
	t.Parallel()

	output := `Load command 1
      cmd LC_SEGMENT_64
  cmdsize 472
  segname __TEXT
   vmaddr 0x0000000100000000
   vmsize 0x0000000000004000
  fileoff 0
 filesize 16384
  maxprot 0x00000005
 initprot 0x00000005
   nsects 5
    flags 0x0
Section
  sectname __text
   segname __TEXT
      addr 0x0000000100001234
      size 0x0000000000001000
Section
  sectname __stubs
   segname __TEXT
      addr 0x0000000100002234
      size 0x0000000000000100
`
	sections := parseMachOSections(output)
	require.Contains(t, sections, "__text")
	require.Contains(t, sections, "__stubs")
}

func TestFilterInterestingStrings(t *testing.T) {
	t.Parallel()

	output := `libfoo.so.6
some random junk
https://example.com/api/v2
/usr/local/lib/libbar.so
error: something went wrong
v2.3.1
ordinary text
WARNING: deprecated feature
another random line`

	result := filterInterestingStrings(output, 30)
	require.Contains(t, result, "https://example.com/api/v2")
	require.Contains(t, result, "/usr/local/lib/libbar.so")
	require.Contains(t, result, "error: something went wrong")
	require.Contains(t, result, "v2.3.1")
	require.Contains(t, result, "WARNING: deprecated feature")
}

func TestFilterInterestingStrings_Limit(t *testing.T) {
	t.Parallel()

	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("https://example.com/path/%d", i))
	}
	output := strings.Join(lines, "\n")

	result := filterInterestingStrings(output, 5)
	require.LessOrEqual(t, len(result), 5)
}

func TestFilterInterestingStrings_SkipsLongLines(t *testing.T) {
	t.Parallel()

	longLine := "https://example.com/" + strings.Repeat("a", 200)
	output := longLine + "\nhttps://short.url\n"

	result := filterInterestingStrings(output, 30)
	require.NotContains(t, result, longLine)
	require.Contains(t, result, "https://short.url")
}

// --- Mock exec pattern tests ---

func TestRunTool_MissingTool(t *testing.T) {
	t.Parallel()

	// A tool that definitely doesn't exist.
	result := runTool(context.Background(), "crush-nonexistent-tool-xyz", "--version")
	require.Empty(t, result)
}

func TestRunTool_AvailableTool(t *testing.T) {
	t.Parallel()

	// Use ls for a real tool invocation with platform gate.
	lsPath, err := exec.LookPath("ls")
	if err != nil {
		t.Skip("ls not available")
	}
	result := runTool(context.Background(), lsPath, "/dev/null")
	require.Contains(t, result, "/dev/null")
}

// TestExecutableExplorer_AllToolsMissing verifies that when no external tools
// are available, the explorer still produces a useful summary with magic
// bytes and file size. CI must NOT require readelf/objdump/otool/nm/strings/
// file installed.
func TestExecutableExplorer_AllToolsMissing(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)
	explorer := &ExecutableExplorer{}

	// The synthetic ELF won't be recognized by `file` as a real executable,
	// and readelf/nm/strings may fail on it. The explorer should still
	// produce basic output.
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "program",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Executable file: program")
	require.Contains(t, s, "Format: ELF")
	require.Contains(t, s, "Size: 64 bytes")
}

// TestExecutableExplorer_DOSCom verifies DOS COM format detection.
func TestExecutableExplorer_DOSCom(t *testing.T) {
	t.Parallel()

	content := []byte{0xEB, 0xFE} // Typical COM start (JMP $).
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "hello.com",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: DOS COM")
}

// TestExecutableExplorer_KernelModule verifies kernel module detection.
func TestExecutableExplorer_KernelModule(t *testing.T) {
	t.Parallel()

	content := buildSyntheticELF(t)
	explorer := &ExecutableExplorer{}

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "mydriver.ko",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "executable", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Format: ELF (ko)")
}

// --- Disambiguate tests ---

func TestDisambiguateCafebabe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name: "fat binary with 2 architectures",
			content: func() []byte {
				data := make([]byte, 8)
				copy(data, []byte{0xCA, 0xFE, 0xBA, 0xBE})
				binary.BigEndian.PutUint32(data[4:], 2)
				return data
			}(),
			expected: "Mach-O Universal",
		},
		{
			name: "fat binary with 4 architectures",
			content: func() []byte {
				data := make([]byte, 8)
				copy(data, []byte{0xCA, 0xFE, 0xBA, 0xBE})
				binary.BigEndian.PutUint32(data[4:], 4)
				return data
			}(),
			expected: "Mach-O Universal",
		},
		{
			name: "Java class major version 52 (Java 8)",
			content: func() []byte {
				data := make([]byte, 8)
				copy(data, []byte{0xCA, 0xFE, 0xBA, 0xBE})
				binary.BigEndian.PutUint32(data[4:], 52) // minor=0, major=52
				return data
			}(),
			expected: "Java class",
		},
		{
			name: "Java class major version 61 (Java 17)",
			content: func() []byte {
				data := make([]byte, 8)
				copy(data, []byte{0xCA, 0xFE, 0xBA, 0xBE})
				// major=61 at offset 6-7, minor=0 at offset 4-5.
				binary.BigEndian.PutUint32(data[4:], 61)
				return data
			}(),
			expected: "Java class",
		},
		{
			name:     "too short for disambiguation",
			content:  []byte{0xCA, 0xFE, 0xBA, 0xBE},
			expected: "Mach-O Universal or Java class (ambiguous)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := disambiguateCafebabe(tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}
