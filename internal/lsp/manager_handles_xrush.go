package lsp

import (
	"log/slog"
	"path/filepath"
	"strings"

	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	powernap "github.com/charmbracelet/x/powernap/pkg/lsp"
)

// NOTE: Body copied from handles()/handlesFiletype(). Keep in sync with base
// version.

func handleFiletypeWithPatterns(sname string, fileTypes []string, matchPatterns []string, filePath string) bool {
	if len(fileTypes) == 0 && len(matchPatterns) == 0 {
		return true
	}

	kind := powernap.DetectLanguage(filePath)
	name := strings.ToLower(filepath.Base(filePath))
	for _, filetype := range fileTypes {
		suffix := strings.ToLower(filetype)
		if !strings.HasPrefix(suffix, ".") {
			suffix = "." + suffix
		}
		if strings.HasSuffix(name, suffix) || filetype == string(kind) {
			slog.Debug("Handles file", "name", sname, "file", name, "filetype", filetype, "kind", kind)
			return true
		}
	}

	if len(matchPatterns) > 0 {
		matcher := NewNamePathMatcher(matchPatterns)
		if matcher.Match(filePath) {
			slog.Debug("Handles file (pattern)", "name", sname, "file", filePath, "patterns", matchPatterns)
			return true
		}
	}

	slog.Debug("Doesn't handle file", "name", sname, "file", name)
	return false
}

func handlesWithPatterns(server *powernapconfig.ServerConfig, matchPatterns []string, filePath, workDir string) bool {
	return handleFiletypeWithPatterns(server.Command, server.FileTypes, matchPatterns, filePath) &&
		hasRootMarkers(workDir, server.RootMarkers)
}
