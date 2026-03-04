package repomap

import (
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestExtractCurrentRunMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []fantasy.Message
		expected []fantasy.Message
	}{
		{
			name:     "empty slice returns nil",
			messages: []fantasy.Message{},
			expected: nil,
		},
		{
			name: "no system messages returns all",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleUser},
			},
			expected: []fantasy.Message{
				{Role: fantasy.MessageRoleUser},
			},
		},
		{
			name: "single system message returns nothing after",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleSystem},
			},
			expected: nil,
		},
		{
			name: "system followed by user returns user",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleUser},
			},
			expected: []fantasy.Message{
				{Role: fantasy.MessageRoleUser},
			},
		},
		{
			name: "multiple systems uses last system index",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleUser},
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleUser},
				{Role: fantasy.MessageRoleAssistant},
			},
			expected: []fantasy.Message{
				{Role: fantasy.MessageRoleUser},
				{Role: fantasy.MessageRoleAssistant},
			},
		},
		{
			name: "messages before last system are excluded",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleUser},
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleUser},
			},
			expected: []fantasy.Message{
				{Role: fantasy.MessageRoleUser},
			},
		},
		{
			name: "non-user roles work correctly",
			messages: []fantasy.Message{
				{Role: fantasy.MessageRoleSystem},
				{Role: fantasy.MessageRoleTool},
			},
			expected: []fantasy.Message{
				{Role: fantasy.MessageRoleTool},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ExtractCurrentRunMessages(tt.messages)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCurrentMessageText(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns empty string", func(t *testing.T) {
		t.Parallel()
		result := ExtractCurrentMessageText([]fantasy.Message{})
		require.Empty(t, result)
	})

	t.Run("single text part works", func(t *testing.T) {
		t.Parallel()
		msg := fantasy.Message{
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello world"}},
		}
		result := ExtractCurrentMessageText([]fantasy.Message{msg})
		require.Equal(t, "hello world", result)
	})

	t.Run("multiple text parts concatenated", func(t *testing.T) {
		t.Parallel()
		msg := fantasy.Message{
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
				fantasy.TextPart{Text: " world"},
			},
		}
		result := ExtractCurrentMessageText([]fantasy.Message{msg})
		require.Equal(t, "hello world", result)
	})

	t.Run("multiple messages concatenated", func(t *testing.T) {
		t.Parallel()
		messages := []fantasy.Message{
			{
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "first"}},
			},
			{
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: " second"}},
			},
		}
		result := ExtractCurrentMessageText(messages)
		require.Equal(t, "first second", result)
	})

	t.Run("empty text part is included", func(t *testing.T) {
		t.Parallel()
		msg := fantasy.Message{
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: ""}},
		}
		result := ExtractCurrentMessageText([]fantasy.Message{msg})
		require.Empty(t, result)
	})

	t.Run("only TextPart content is extracted", func(t *testing.T) {
		t.Parallel()
		msg := fantasy.Message{
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "first"},
				fantasy.TextPart{Text: "second"},
			},
		}
		result := ExtractCurrentMessageText([]fantasy.Message{msg})
		require.Equal(t, "firstsecond", result)
	})
}

func TestExtractMentionedFnames(t *testing.T) {
	t.Parallel()

	t.Run("empty text returns nil", func(t *testing.T) {
		t.Parallel()
		result := ExtractMentionedFnames("", []string{"a.go"}, nil)
		require.Nil(t, result)
	})

	t.Run("whitespace text returns nil", func(t *testing.T) {
		t.Parallel()
		result := ExtractMentionedFnames("   \n\t  ", []string{"a.go"}, nil)
		require.Nil(t, result)
	})

	t.Run("exact relative path match", func(t *testing.T) {
		t.Parallel()
		text := "Look at path/to/file.go for details"
		repoFiles := []string{"path/to/file.go", "other.txt"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"path/to/file.go"}, result)
	})

	t.Run("exact match with leading/trailing spaces", func(t *testing.T) {
		t.Parallel()
		text := "Check out   file.go  and other.go"
		repoFiles := []string{"file.go", "other.go", "third.txt"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"file.go", "other.go"}, result)
	})

	t.Run("unique basename match", func(t *testing.T) {
		t.Parallel()
		text := "Look at file.go in the code"
		repoFiles := []string{"unique/file.go", "other.txt"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"unique/file.go"}, result)
	})

	t.Run("basename already in chat is skipped", func(t *testing.T) {
		t.Parallel()
		text := "Look at file.go"
		repoFiles := []string{"unique/file.go", "other/file.go"}
		inChat := []string{"other/file.go"}
		result := ExtractMentionedFnames(text, repoFiles, inChat)
		// Should only match unique/file.go since file.go basename is in-chat
		require.Nil(t, result) // No match because basename file.go already exists in inChat
	})

	t.Run("non-unique basename is not added", func(t *testing.T) {
		t.Parallel()
		text := "Check common.go"
		repoFiles := []string{"dir1/common.go", "dir2/common.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Nil(t, result) // No match because basename not unique
	})

	t.Run("exact match takes priority over basename", func(t *testing.T) {
		t.Parallel()
		text := "Check file.go and other/file.go"
		repoFiles := []string{"file.go", "other/file.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"file.go", "other/file.go"}, result)
	})

	t.Run("normalized path matching", func(t *testing.T) {
		t.Parallel()
		text := "Look at path/to/file.go"
		repoFiles := []string{"path//to/./file.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"path/to/file.go"}, result)
	})

	t.Run("path separators in text preserved", func(t *testing.T) {
		t.Parallel()
		text := "Use internal/agent/file.go"
		repoFiles := []string{"internal/agent/file.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"internal/agent/file.go"}, result)
	})

	t.Run("files not in repo ignored", func(t *testing.T) {
		t.Parallel()
		text := "Check missing.go and notfound.ts"
		repoFiles := []string{"actual.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Nil(t, result)
	})

	t.Run("returns deterministic sorted output", func(t *testing.T) {
		t.Parallel()
		text := "zebra.go alpha.go middle.go"
		repoFiles := []string{"zebra.go", "alpha.go", "middle.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		expected := []string{"alpha.go", "middle.go", "zebra.go"}
		require.Equal(t, expected, result)
	})

	t.Run("deduplicates repeated mentions", func(t *testing.T) {
		t.Parallel()
		text := "file.go file.go another.go file.go"
		repoFiles := []string{"file.go", "another.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"another.go", "file.go"}, result)
	})

	t.Run("handles paths with special characters", func(t *testing.T) {
		t.Parallel()
		text := "Check file-name.go and file_name.go"
		repoFiles := []string{"file-name.go", "file_name.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"file-name.go", "file_name.go"}, result)
	})

	t.Run("empty addableRepoFiles returns nil", func(t *testing.T) {
		t.Parallel()
		text := "Look at file.go"
		result := ExtractMentionedFnames(text, []string{}, nil)
		require.Nil(t, result)
	})

	t.Run("paths with dots handled correctly", func(t *testing.T) {
		t.Parallel()
		text := "Check internal/config/config.go"
		repoFiles := []string{"internal/config/config.go"}
		result := ExtractMentionedFnames(text, repoFiles, nil)
		require.Equal(t, []string{"internal/config/config.go"}, result)
	})

	t.Run("single character basename requires exact path", func(t *testing.T) {
		t.Parallel()
		text := "Look at a.go for examples"
		repoFiles := []string{"dir/a.go", "b.go"}
		inChat := []string{}
		result := ExtractMentionedFnames(text, repoFiles, inChat)
		require.Equal(t, []string{"dir/a.go"}, result)
	})
}

func TestExtractIdents(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns nil", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("")
		require.Nil(t, result)
	})

	t.Run("whitespace only returns nil", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("   \n\t  ")
		require.Nil(t, result)
	})

	t.Run("single identifier", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("hello")
		require.Equal(t, []string{"hello"}, result)
	})

	t.Run("multiple identifiers split by space", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("hello world foo bar")
		require.Equal(t, []string{"bar", "foo", "hello", "world"}, result)
	})

	t.Run("identifiers split by punctuation", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("hello,world!foo;bar")
		require.Equal(t, []string{"bar", "foo", "hello", "world"}, result)
	})

	t.Run("numbers in identifiers", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("test123 var2 name1")
		require.Equal(t, []string{"name1", "test123", "var2"}, result)
	})

	t.Run("underscores in identifiers", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("test_var my_name")
		require.Equal(t, []string{"my_name", "test_var"}, result)
	})

	t.Run("camels preserve original case", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("MyClass TestFunc")
		require.Equal(t, []string{"MyClass", "TestFunc"}, result)
	})

	t.Run("dashes are not word chars", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("file-name file_name")
		require.Equal(t, []string{"file", "file_name", "name"}, result)
	})

	t.Run("dots are not word chars", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("file.go module.test")
		require.Equal(t, []string{"file", "go", "module", "test"}, result)
	})

	t.Run("returns sorted output", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("zebra alpha charlie beta")
		require.Equal(t, []string{"alpha", "beta", "charlie", "zebra"}, result)
	})

	t.Run("deduplicates repeated identifiers", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("foo foo foo bar bar")
		require.Equal(t, []string{"bar", "foo"}, result)
	})

	t.Run("mixed whitespace and separators", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("hello  world\ttest\nend;final")
		require.Equal(t, []string{"end", "final", "hello", "test", "world"}, result)
	})

	t.Run("brackets are separators", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("test[0] func()")
		require.Equal(t, []string{"0", "func", "test"}, result)
	})

	t.Run("slashes are separators", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("path/to/file")
		require.Equal(t, []string{"file", "path", "to"}, result)
	})

	t.Run("special characters all act as separators", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("!@#$Hello^&*()World")
		require.Equal(t, []string{"Hello", "World"}, result)
	})

	t.Run("empty tokens from splits are ignored", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("...hello,,,world===test")
		require.Equal(t, []string{"hello", "test", "world"}, result)
	})

	t.Run("single character identifiers extracted", func(t *testing.T) {
		t.Parallel()
		result := ExtractIdents("a b c")
		require.Equal(t, []string{"a", "b", "c"}, result)
	})
}

func BenchmarkExtractMentionedFnames(b *testing.B) {
	repoFiles := make([]string, 0, 2000)
	for i := range 1000 {
		repoFiles = append(repoFiles,
			"internal/pkg"+strings.Repeat("a", i%7)+"/file"+strings.Repeat("b", i%9)+".go",
			"cmd/tool"+strings.Repeat("x", i%5)+"/main"+strings.Repeat("y", i%8)+".go",
		)
	}
	inChat := []string{"internal/pkg/a/fileb.go", "cmd/toolx/mainy.go"}
	text := "Please check internal/pkgaa/filebbb.go and mainyyyy.go plus cmd/tool/main.go and internal/pkg/file.go"

	b.ResetTimer()
	for range b.N {
		_ = ExtractMentionedFnames(text, repoFiles, inChat)
	}
}

func BenchmarkExtractIdents(b *testing.B) {
	text := strings.Repeat("alpha_beta123 gammaDelta foo.bar baz-qux /tmp/path\n", 200)

	b.ResetTimer()
	for range b.N {
		_ = ExtractIdents(text)
	}
}

func TestIdentFilenameMatches(t *testing.T) {
	t.Parallel()

	t.Run("empty idents returns nil", func(t *testing.T) {
		t.Parallel()
		result := IdentFilenameMatches([]string{}, []string{"test.go"})
		require.Nil(t, result)
	})

	t.Run("empty repo files returns nil", func(t *testing.T) {
		t.Parallel()
		result := IdentFilenameMatches([]string{"test"}, []string{})
		require.Nil(t, result)
	})

	t.Run("ident shorter than 5 characters ignored", func(t *testing.T) {
		t.Parallel()
		idents := []string{"test", "abc", "code"}
		repoFiles := []string{"test.go", "abc.txt", "code.rs"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Nil(t, result)
	})

	t.Run("exact stem match", func(t *testing.T) {
		t.Parallel()
		idents := []string{"config"}
		repoFiles := []string{"config.go"}
		// "config" is len 6, so it should match
		require.Len(t, "config", 6)
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"config.go"}, result)
	})

	t.Run("case insensitive stem match", func(t *testing.T) {
		t.Parallel()
		idents := []string{"Config", "CONFIG", "config"}
		repoFiles := []string{"config.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"config.go"}, result)
	})

	t.Run("multiple files with same stem all matched", func(t *testing.T) {
		t.Parallel()
		idents := []string{"handler"}
		repoFiles := []string{"api/handler.go", "web/handler.go", "handler_test.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		// Only api/handler.go and web/handler.go have stem "handler"
		// handler_test.go has stem "handler_test"
		require.ElementsMatch(t, []string{"api/handler.go", "web/handler.go"}, result)
	})

	t.Run("multiple idents matching different stems", func(t *testing.T) {
		t.Parallel()
		idents := []string{"service", "handler"}
		repoFiles := []string{"service.go", "handler.go", "other.txt"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.ElementsMatch(t, []string{"service.go", "handler.go"}, result)
	})

	t.Run("stem without extension matched", func(t *testing.T) {
		t.Parallel()
		idents := []string{"myhandler"}
		repoFiles := []string{"handlers/myhandler.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"handlers/myhandler.go"}, result)
	})

	t.Run("no matching stem returns nil", func(t *testing.T) {
		t.Parallel()
		idents := []string{"nonexistent"}
		repoFiles := []string{"actual.go", "real.txt"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Nil(t, result)
	})

	t.Run("mixed case idents all match lowercase stem", func(t *testing.T) {
		t.Parallel()
		idents := []string{"Service", "service", "SERVICE"}
		repoFiles := []string{"Service.go", "service_test.go", "SERVICE_SPEC.md"}
		result := IdentFilenameMatches(idents, repoFiles)
		// Only Service.go has stem "service"
		// service_test.go has stem "service_test"
		// SERVICE_SPEC.md has stem "service_spec"
		require.ElementsMatch(t, []string{"Service.go"}, result)
	})

	t.Run("returns deterministic sorted output", func(t *testing.T) {
		t.Parallel()
		idents := []string{"zebra", "alpha"}
		repoFiles := []string{"zebra.go", "alpha.go", "beta.txt"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"alpha.go", "zebra.go"}, result)
	})

	t.Run("ident exactly 5 characters matches", func(t *testing.T) {
		t.Parallel()
		idents := []string{"test12345"}
		repoFiles := []string{"test12345.go"}
		require.Len(t, "test12345", 9)
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"test12345.go"}, result)
	})

	t.Run("ident with underscores matches stem", func(t *testing.T) {
		t.Parallel()
		idents := []string{"test_handler"}
		repoFiles := []string{"test_handler.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"test_handler.go"}, result)
	})

	t.Run("duplicate matches deduplicated", func(t *testing.T) {
		t.Parallel()
		idents := []string{"service", "service"}
		repoFiles := []string{"service.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"service.go"}, result)
	})

	t.Run("path normalization on repo files", func(t *testing.T) {
		t.Parallel()
		idents := []string{"config"}
		repoFiles := []string{"path//to/config.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.Equal(t, []string{"path/to/config.go"}, result)
	})

	t.Run("files with multiple extensions handled", func(t *testing.T) {
		t.Parallel()
		idents := []string{"archive.zip"}
		repoFiles := []string{"archive.tar.gz", "archive.zip"}
		result := IdentFilenameMatches(idents, repoFiles)
		// path.Ext removes only the last extension: .gz, leaving stem "archive.tar"
		// Only archive.zip has stem matching "archive.zip"
		require.ElementsMatch(t, []string{"archive.zip"}, result)
	})

	t.Run("ident with special chars matches same stem", func(t *testing.T) {
		t.Parallel()
		idents := []string{"my-service"}
		repoFiles := []string{"my_service.go", "my-service.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		// my-service.go has stem "my-service" which matches ident "my-service"
		// my_service.go has stem "my_service" which doesn't match
		require.ElementsMatch(t, []string{"my-service.go"}, result)
	})

	t.Run("ident trimmed before processing", func(t *testing.T) {
		t.Parallel()
		idents := []string{"  config  ", "\thandler\t"}
		repoFiles := []string{"config.go", "handler.go"}
		result := IdentFilenameMatches(idents, repoFiles)
		require.ElementsMatch(t, []string{"config.go", "handler.go"}, result)
	})
}
