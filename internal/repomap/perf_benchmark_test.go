package repomap

import (
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
)

func BenchmarkStringInternerIntern(b *testing.B) {
	values := []string{
		"src/main.go",
		"src/service.go",
		"def",
		"ref",
		"function",
		"call",
		"go",
		"User",
		"Run",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		interner := newStringInterner(len(values))
		for _, v := range values {
			_ = interner.Intern(v)
			_ = interner.Intern(v)
		}
	}
}

func BenchmarkSortTagsDeterministic(b *testing.B) {
	tags := make([]treesitter.Tag, 0, 256)
	for i := range 256 {
		tags = append(tags, treesitter.Tag{
			RelPath:  "src/file.go",
			Name:     "Name",
			Kind:     "def",
			Line:     (i % 100) + 1,
			Language: "go",
			NodeType: "function",
		})
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		working := append([]treesitter.Tag(nil), tags...)
		sortTagsDeterministic(working)
	}
}
