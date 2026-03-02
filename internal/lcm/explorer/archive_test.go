package explorer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArchiveExplorer_CanHandle_Extensions(t *testing.T) {
	t.Parallel()

	explorer := &ArchiveExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		// All 28 archive extensions.
		{name: "zip", path: "archive.zip", expected: true},
		{name: "tar", path: "archive.tar", expected: true},
		{name: "gz", path: "archive.gz", expected: true},
		{name: "tgz", path: "archive.tgz", expected: true},
		{name: "bz2", path: "archive.bz2", expected: true},
		{name: "xz", path: "archive.xz", expected: true},
		{name: "lz", path: "archive.lz", expected: true},
		{name: "lz4", path: "archive.lz4", expected: true},
		{name: "zst", path: "archive.zst", expected: true},
		{name: "7z", path: "archive.7z", expected: true},
		{name: "rar", path: "archive.rar", expected: true},
		{name: "cab", path: "archive.cab", expected: true},
		{name: "ar", path: "archive.ar", expected: true},
		{name: "deb", path: "package.deb", expected: true},
		{name: "rpm", path: "package.rpm", expected: true},
		{name: "cpio", path: "archive.cpio", expected: true},
		{name: "iso", path: "image.iso", expected: true},
		{name: "dmg", path: "image.dmg", expected: true},
		{name: "wim", path: "image.wim", expected: true},
		{name: "jar", path: "app.jar", expected: true},
		{name: "war", path: "app.war", expected: true},
		{name: "ear", path: "app.ear", expected: true},
		{name: "apk", path: "app.apk", expected: true},
		{name: "ipa", path: "app.ipa", expected: true},
		{name: "nupkg", path: "pkg.nupkg", expected: true},
		{name: "crx", path: "ext.crx", expected: true},
		{name: "xpi", path: "ext.xpi", expected: true},
		{name: "vsix", path: "ext.vsix", expected: true},

		// Double extensions.
		{name: "tar.gz", path: "archive.tar.gz", expected: true},
		{name: "tar.bz2", path: "archive.tar.bz2", expected: true},
		{name: "tar.xz", path: "archive.tar.xz", expected: true},
		{name: "tar.lz", path: "archive.tar.lz", expected: true},
		{name: "tar.lz4", path: "archive.tar.lz4", expected: true},
		{name: "tar.zst", path: "archive.tar.zst", expected: true},

		// Case insensitivity.
		{name: "ZIP uppercase", path: "archive.ZIP", expected: true},
		{name: "Tar.Gz mixed case", path: "archive.Tar.Gz", expected: true},

		// Non-archive extensions.
		{name: "json not archive", path: "data.json", expected: false},
		{name: "go not archive", path: "main.go", expected: false},
		{name: "txt not archive", path: "readme.txt", expected: false},
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

func TestArchiveExplorer_CanHandle_MagicBytes(t *testing.T) {
	t.Parallel()

	explorer := &ArchiveExplorer{}

	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "ZIP magic",
			content:  append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "gzip magic",
			content:  append([]byte{0x1f, 0x8b}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "RAR magic",
			content:  append([]byte("Rar!\x1a\x07"), bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "7z magic",
			content:  append([]byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "XZ magic",
			content:  append([]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "bzip2 magic",
			content:  append([]byte("BZ"), bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "LZ4 magic",
			content:  append([]byte{0x04, 0x22, 0x4d, 0x18}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "zstd magic",
			content:  append([]byte{0x28, 0xb5, 0x2f, 0xfd}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "deb ar magic",
			content:  append([]byte("!<arch>\n"), bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name:     "RPM magic",
			content:  append([]byte{0xed, 0xab, 0xee, 0xdb}, bytes.Repeat([]byte{0}, 100)...),
			expected: true,
		},
		{
			name: "TAR magic at offset 257",
			content: func() []byte {
				data := make([]byte, 512)
				copy(data[257:], "ustar")
				return data
			}(),
			expected: true,
		},
		{
			name:     "no magic",
			content:  bytes.Repeat([]byte{0x42}, 512),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Use a path with no recognized extension.
			result := explorer.CanHandle("unknownfile", tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestArchiveExplorer_Explore_ZIP(t *testing.T) {
	t.Parallel()

	zipData := createTestZIP(t, map[string][]byte{
		"README.md":          []byte("# Hello\n"),
		"src/main.go":        []byte("package main\n\nfunc main() {}\n"),
		"src/util.go":        []byte("package main\n"),
		"src/data/config.go": []byte("package data\n"),
		"LICENSE":            []byte("MIT License\n"),
	})

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "project.zip",
		Content: zipData,
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)

	s := result.Summary
	require.Contains(t, s, "Archive file: project.zip")
	require.Contains(t, s, "Format: zip")
	require.Contains(t, s, "Files: 5")
	require.Contains(t, s, "Total uncompressed:")
	require.Contains(t, s, ".go: 3")
	require.Contains(t, s, ".md: 1")
}

func TestArchiveExplorer_Explore_TAR(t *testing.T) {
	t.Parallel()

	tarData := createTestTAR(t, map[string][]byte{
		"file1.txt":     []byte("hello world\n"),
		"dir/file2.txt": []byte("nested file\n"),
	})

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "archive.tar",
		Content: tarData,
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Archive file: archive.tar")
	require.Contains(t, s, "Format: tar")
	require.Contains(t, s, "Files: 2")
	require.Contains(t, s, ".txt: 2")
}

func TestArchiveExplorer_Explore_TARGz(t *testing.T) {
	t.Parallel()

	tarData := createTestTAR(t, map[string][]byte{
		"app/main.py":   []byte("print('hello')\n"),
		"app/utils.py":  []byte("def helper(): pass\n"),
		"app/README.md": []byte("# App\n"),
	})

	// Gzip the tar data.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err := gw.Write(tarData)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "archive.tar.gz",
		Content: gzBuf.Bytes(),
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Archive file: archive.tar.gz")
	require.Contains(t, s, "Format: tar.gz")
	require.Contains(t, s, "Files: 3")
	require.Contains(t, s, ".py: 2")
	require.Contains(t, s, ".md: 1")
}

func TestArchiveExplorer_Explore_JAR_Manifest(t *testing.T) {
	t.Parallel()

	manifest := []byte("Manifest-Version: 1.0\r\nMain-Class: com.example.Main\r\nCreated-By: 1.8 (Oracle)\r\n")
	zipData := createTestZIP(t, map[string][]byte{
		"META-INF/MANIFEST.MF":    manifest,
		"com/example/Main.class":  {0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00},
		"com/example/Utils.class": {0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00},
	})

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "app.jar",
		Content: zipData,
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Format: jar")
	require.Contains(t, s, "MANIFEST.MF")
	require.Contains(t, s, "Manifest-Version: 1.0")
	require.Contains(t, s, "Main-Class: com.example.Main")
}

func TestArchiveExplorer_Explore_Deb(t *testing.T) {
	t.Parallel()

	debData := createTestDeb(t)

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "package.deb",
		Content: debData,
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Archive file: package.deb")
	require.Contains(t, s, "Format: deb (ar archive)")
	require.Contains(t, s, "Members:")
	require.Contains(t, s, "debian-binary")
}

func TestArchiveExplorer_Explore_RPM(t *testing.T) {
	t.Parallel()

	rpmData := createTestRPM(t)

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "package.rpm",
		Content: rpmData,
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Archive file: package.rpm")
	require.Contains(t, s, "Format: RPM package")
	require.Contains(t, s, "RPM version: 3.0")
	require.Contains(t, s, "Type: binary")
	require.Contains(t, s, "Package name: test-package")
}

func TestArchiveExplorer_Explore_Opaque(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		format string
	}{
		{name: "7z", path: "archive.7z", format: "7z"},
		{name: "rar", path: "archive.rar", format: "rar"},
		{name: "iso", path: "disk.iso", format: "iso"},
		{name: "dmg", path: "disk.dmg", format: "dmg"},
		{name: "cab", path: "setup.cab", format: "cab"},
		{name: "wim", path: "image.wim", format: "wim"},
		{name: "cpio", path: "archive.cpio", format: "cpio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			explorer := &ArchiveExplorer{}
			result, err := explorer.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: []byte("dummy opaque content"),
			})
			// Opaque formats return nil error, not chain fallthrough.
			require.NoError(t, err)
			require.Equal(t, "archive", result.ExplorerUsed)

			s := result.Summary
			require.Contains(t, s, "Archive file:")
			require.Contains(t, s, tt.format)
			require.Contains(t, s, "cannot be listed without external tools")
		})
	}
}

func TestArchiveExplorer_Explore_ZIP_Encrypted(t *testing.T) {
	t.Parallel()

	// Create a ZIP with the encrypted flag set.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Create a file with the encrypted flag manually set.
	hdr := &zip.FileHeader{
		Name:   "secret.txt",
		Flags:  0x1, // encrypted bit
		Method: zip.Store,
	}
	fw, err := w.CreateHeader(hdr)
	require.NoError(t, err)
	_, err = fw.Write([]byte("secret"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "encrypted.zip",
		Content: buf.Bytes(),
	})
	require.NoError(t, err)

	require.Contains(t, result.Summary, "Encrypted: yes")
}

func TestArchiveExplorer_Explore_Enhancement_TimesAndMethods(t *testing.T) {
	t.Parallel()

	// Create a ZIP with files at different modification times.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	early := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	hdr1 := &zip.FileHeader{
		Name:     "old.txt",
		Method:   zip.Store,
		Modified: early,
	}
	fw1, err := w.CreateHeader(hdr1)
	require.NoError(t, err)
	_, err = fw1.Write([]byte("old file"))
	require.NoError(t, err)

	hdr2 := &zip.FileHeader{
		Name:     "new.txt",
		Method:   zip.Deflate,
		Modified: late,
	}
	fw2, err := w.CreateHeader(hdr2)
	require.NoError(t, err)
	_, err = fw2.Write([]byte("new file content that should be compressed"))
	require.NoError(t, err)

	require.NoError(t, w.Close())

	explorer := &ArchiveExplorer{formatterProfile: OutputProfileEnhancement}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "test.zip",
		Content: buf.Bytes(),
	})
	require.NoError(t, err)

	s := result.Summary
	require.Contains(t, s, "Modification time range:")
	require.Contains(t, s, "Compression methods:")
	require.Contains(t, s, "Store:")
	require.Contains(t, s, "Deflate:")
}

func TestArchiveExplorer_Explore_GzipStandalone(t *testing.T) {
	t.Parallel()

	// Create a gzip file containing non-tar data.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte("this is just plain gzipped text, not a tar archive"))
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "file.gz",
		Content: buf.Bytes(),
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	require.Contains(t, s, "Format: gzip")
	require.Contains(t, s, "Compressed size:")
	require.Contains(t, s, "Uncompressed size:")
}

func TestArchiveExplorer_Explore_GzipWithTar(t *testing.T) {
	t.Parallel()

	// Create a gzip file containing tar data (detected via .gz extension).
	tarData := createTestTAR(t, map[string][]byte{
		"hello.txt": []byte("world\n"),
	})

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err := gw.Write(tarData)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	explorer := &ArchiveExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "archive.gz",
		Content: gzBuf.Bytes(),
	})
	require.NoError(t, err)
	require.Equal(t, "archive", result.ExplorerUsed)

	s := result.Summary
	// Should detect tar inside gzip.
	require.Contains(t, s, "Format: tar.gz")
	require.Contains(t, s, "Files: 1")
}

func TestArchiveExplorer_FormatSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 bytes"},
		{512, "512 bytes"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, formatSize(tt.bytes))
		})
	}
}

func TestArchiveExplorer_DetectArchiveMagic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "ZIP",
			content:  []byte("PK\x03\x04rest"),
			expected: "zip",
		},
		{
			name:     "gzip",
			content:  []byte{0x1f, 0x8b, 0x08},
			expected: "gzip",
		},
		{
			name:     "RAR",
			content:  []byte("Rar!\x1a\x07\x00"),
			expected: "rar",
		},
		{
			name: "TAR ustar",
			content: func() []byte {
				data := make([]byte, 512)
				copy(data[257:], "ustar")
				return data
			}(),
			expected: "tar",
		},
		{
			name:     "empty",
			content:  []byte{},
			expected: "",
		},
		{
			name:     "unknown",
			content:  []byte("hello world"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := detectArchiveMagic(tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

// createTestZIP creates a synthetic ZIP archive from a map of filenames to
// contents.
func createTestZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// createTestTAR creates a synthetic uncompressed tar archive from a map of
// filenames to contents.
func createTestTAR(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		err := tw.WriteHeader(&tar.Header{
			Name:    name,
			Size:    int64(len(content)),
			Mode:    0o644,
			ModTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			Uname:   "testuser",
		})
		require.NoError(t, err)
		_, err = tw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// createTestDeb creates a synthetic .deb file (ar archive format) with
// standard debian members.
func createTestDeb(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	// Global header.
	buf.WriteString("!<arch>\n")

	// Add a member: debian-binary
	content := []byte("2.0\n")
	writeArMember(&buf, "debian-binary", content)

	// Add a member: control.tar.gz (dummy)
	controlData := []byte("dummy control data")
	writeArMember(&buf, "control.tar.gz", controlData)

	// Add a member: data.tar.gz (dummy)
	dataData := []byte("dummy data payload")
	writeArMember(&buf, "data.tar.gz", dataData)

	return buf.Bytes()
}

// writeArMember writes a single ar member header + content.
func writeArMember(buf *bytes.Buffer, name string, content []byte) {
	// ar member header: name[16] + mtime[12] + uid[6] + gid[6] + mode[8] +
	// size[10] + magic[2]
	header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n",
		name+"/", "0", "0", "0", "100644", len(content))
	buf.WriteString(header)
	buf.Write(content)
	// Pad to 2-byte boundary.
	if len(content)%2 != 0 {
		buf.WriteByte('\n')
	}
}

// createTestRPM creates a synthetic RPM file with a 96-byte lead.
func createTestRPM(t *testing.T) []byte {
	t.Helper()

	data := make([]byte, 96)
	// Magic: 0xedabeedb.
	data[0] = 0xed
	data[1] = 0xab
	data[2] = 0xee
	data[3] = 0xdb
	// Version: 3.0.
	data[4] = 3
	data[5] = 0
	// Type: 0 = binary (big-endian uint16).
	data[6] = 0
	data[7] = 0
	// Arch at offset 8-9 (not parsed, skip).
	// Name at offset 10, null-terminated.
	copy(data[10:], "test-package")

	return data
}
