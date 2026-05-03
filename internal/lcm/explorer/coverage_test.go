package explorer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Tests for previously uncovered explorer Explore methods.

func TestFallbackExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	e := &FallbackExplorer{}
	require.True(t, e.CanHandle("anything", nil))
}

func TestFallbackExplorer_Text(t *testing.T) {
	t.Parallel()
	e := &FallbackExplorer{}
	result, err := e.Explore(context.Background(), ExploreInput{
		Path:    "readme.txt",
		Content: []byte("Hello world, this is a text file."),
	})
	require.NoError(t, err)
	require.Equal(t, "fallback", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Hello world")
}

func TestFallbackExplorer_Binary(t *testing.T) {
	t.Parallel()
	e := &FallbackExplorer{}
	content := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	result, err := e.Explore(context.Background(), ExploreInput{
		Path:    "image.dat",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "fallback", result.ExplorerUsed)
	require.Contains(t, result.Summary, "binary")
}

func TestTOMLExplorer_Explore(t *testing.T) {
	t.Parallel()
	e := &TOMLExplorer{}
	content := []byte(`[server]
port = 8080
host = "localhost"

[database]
url = "postgres://localhost/db"
`)
	result, err := e.Explore(context.Background(), ExploreInput{Path: "config.toml", Content: content})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "server")
}

func TestINIExplorer_Explore(t *testing.T) {
	t.Parallel()
	e := &INIExplorer{}
	content := []byte(`[server]
port=8080
host=localhost

[database]
url=postgres://localhost/db
`)
	result, err := e.Explore(context.Background(), ExploreInput{Path: "config.ini", Content: content})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "server")
}

func TestXMLExplorer_Explore(t *testing.T) {
	t.Parallel()
	e := &XMLExplorer{}
	content := []byte(`<?xml version="1.0"?>
<project>
  <name>crush</name>
  <dependencies>
    <dependency>foo</dependency>
    <dependency>bar</dependency>
  </dependencies>
</project>
`)
	result, err := e.Explore(context.Background(), ExploreInput{Path: "pom.xml", Content: content})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "project")
}

func TestHTMLExplorer_Explore(t *testing.T) {
	t.Parallel()
	e := &HTMLExplorer{}
	content := []byte(`<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
  <div class="container">
    <h1>Hello</h1>
    <p>World</p>
  </div>
</body>
</html>
`)
	result, err := e.Explore(context.Background(), ExploreInput{Path: "index.html", Content: content})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "Test Page")
}
