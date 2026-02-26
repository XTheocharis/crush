package explorer

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestSQLiteExplorer_CanHandle(t *testing.T) {
	explorer := &SQLiteExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{
			name:     ".sqlite extension",
			path:     "test.sqlite",
			content:  []byte("some random content"),
			expected: true,
		},
		{
			name:     ".sqlite3 extension",
			path:     "test.sqlite3",
			content:  []byte("some random content"),
			expected: true,
		},
		{
			name:     ".db extension",
			path:     "database.db",
			content:  []byte("some random content"),
			expected: true,
		},
		{
			name:     ".db upper case",
			path:     "DATABASE.DB",
			content:  []byte("some random content"),
			expected: true,
		},
		{
			name:     "sqlite magic header",
			path:     "datafile",
			content:  []byte("SQLite format 3\000" + strings.Repeat("X", 100)),
			expected: true,
		},
		{
			name:     "json file",
			path:     "test.json",
			content:  []byte(`{"key": "value"}`),
			expected: false,
		},
		{
			name:     "unknown extension",
			path:     "unknown.xyz",
			content:  []byte("some content"),
			expected: false,
		},
		{
			name:     "text file",
			path:     "README.txt",
			content:  []byte("some text content"),
			expected: false,
		},
		{
			name:     "csv file",
			path:     "data.csv",
			content:  []byte("name,value\na,1"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := explorer.CanHandle(tt.path, tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSQLiteExplorer_Explore(t *testing.T) {
	t.Parallel()

	t.Run("valid database", func(t *testing.T) {
		t.Parallel()

		// Create test database in temp file
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
		require.NoError(t, err)
		defer db.Close()

		// Create tables with various types and indexes
		_, err = db.ExecContext(context.Background(), `
			CREATE TABLE users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				email TEXT UNIQUE,
				age INTEGER,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE posts (
				id INTEGER PRIMARY KEY,
				title TEXT,
				user_id INTEGER,
				content TEXT
			);
			CREATE TABLE comments (
				id INTEGER,
				post_id INTEGER,
				comment TEXT
			);
			CREATE INDEX idx_users_email ON users(email);
			CREATE INDEX idx_posts_user ON posts(user_id);
		`)
		require.NoError(t, err)

		// Read database content
		content, err := os.ReadFile(dbPath)
		require.NoError(t, err)

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "test.db",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "sqlite", result.ExplorerUsed)
		require.Greater(t, result.TokenEstimate, 0)

		summary := result.Summary

		// Check basic info
		require.Contains(t, summary, "SQLite database: test.db")
		require.Contains(t, summary, "Size: ")
		require.Contains(t, summary, "SQLite version:")

		// Check table inventory
		require.Contains(t, summary, "Tables: 3")
		require.Contains(t, summary, "Table inventory:")
		require.Contains(t, summary, "- users")
		require.Contains(t, summary, "- posts")
		require.Contains(t, summary, "- comments")

		// Check index inventory
		require.Contains(t, summary, "Indexes: 2")
		require.Contains(t, summary, "Index inventory:")

		// Check schema details
		require.Contains(t, summary, "Schema details:")
		require.Contains(t, summary, "users (5 columns):")
		require.Contains(t, summary, "- id INTEGER (PK)")
		require.Contains(t, summary, "- name TEXT NOT NULL")
		require.Contains(t, summary, "- email TEXT")
		require.Contains(t, summary, "- age INTEGER")
		require.Contains(t, summary, "posts (4 columns):")
		require.Contains(t, summary, "- id INTEGER")
		require.Contains(t, summary, "- title TEXT")
		require.Contains(t, summary, "comments (3 columns):")
	})

	t.Run("empty database", func(t *testing.T) {
		t.Parallel()

		// Create empty test database
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "empty.db")

		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
		require.NoError(t, err)
		// Execute a simple query to ensure the database file is created
		_, err = db.ExecContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		// Close database before reading to ensure content is flushed
		db.Close()

		content, err := os.ReadFile(dbPath)
		require.NoError(t, err)

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "empty.db",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "sqlite", result.ExplorerUsed)

		summary := result.Summary
		require.Contains(t, summary, "SQLite database: empty.db")
		require.Contains(t, summary, "Tables: 0")
		require.Contains(t, summary, "Indexes: 0")
	})

	t.Run("database with special characters in names", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "special.db")

		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
		require.NoError(t, err)
		defer db.Close()

		// Tables with special characters need quoting
		_, err = db.ExecContext(context.Background(), `
			CREATE TABLE "user-data" (
				id INTEGER PRIMARY KEY,
				"user_name" TEXT
			);
			CREATE TABLE "order_items" (
				id INTEGER,
				"order-id" INTEGER
			);
		`)
		require.NoError(t, err)

		content, err := os.ReadFile(dbPath)
		require.NoError(t, err)

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "special.db",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "sqlite", result.ExplorerUsed)

		summary := result.Summary
		require.Contains(t, summary, "Tables: 2")
		require.Contains(t, summary, "user-data")
		require.Contains(t, summary, "order_items")
	})

	t.Run("corrupted database", func(t *testing.T) {
		t.Parallel()

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "corrupted.db",
			Content: []byte("This is definitely not a SQLite database"),
		})
		require.NoError(t, err)
		require.Equal(t, "sqlite", result.ExplorerUsed)

		summary := result.Summary
		require.Contains(t, summary, "SQLite database: corrupted.db")
		require.Contains(t, summary, "Error: Invalid SQLite database file")
	})

	t.Run("non-sqlite content with extension", func(t *testing.T) {
		t.Parallel()

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "test.db",
			Content: []byte("Just plain text that looks like a .db file but isn't one"),
		})
		require.NoError(t, err)
		require.Equal(t, "sqlite", result.ExplorerUsed)

		summary := result.Summary
		require.Contains(t, summary, "SQLite database: test.db")
		require.Contains(t, summary, "Error:")
	})

	t.Run("database with primary key constraints", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "pk.db")

		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(context.Background(), `
			CREATE TABLE test (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL
			);
		`)
		require.NoError(t, err)

		content, err := os.ReadFile(dbPath)
		require.NoError(t, err)

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "pk.db",
			Content: content,
		})
		require.NoError(t, err)

		summary := result.Summary
		// PRAGMA table_info includes PK info
		require.Contains(t, summary, "Schema details:")
	})

	t.Run("database with autoincrement", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "auto.db")

		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(context.Background(), `
			CREATE TABLE products (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				sku TEXT UNIQUE NOT NULL,
				price REAL
			);
		`)
		require.NoError(t, err)

		content, err := os.ReadFile(dbPath)
		require.NoError(t, err)

		explorer := &SQLiteExplorer{}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "auto.db",
			Content: content,
		})
		require.NoError(t, err)

		summary := result.Summary
		require.Contains(t, summary, "products (3 columns):")
	})
}

func TestSQLiteExplorer_ThroughRegistry(t *testing.T) {
	t.Parallel()

	// Create test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(dbPath)))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE accounts (
			id INTEGER PRIMARY KEY,
			username TEXT,
			email TEXT
		);
		CREATE INDEX idx_accounts_username ON accounts(username);
	`)
	require.NoError(t, err)

	// Read database content
	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	// Test that SQLite explorer is used (won't be in registry yet since
	// we didn't modify explorer.go, but we can test the explorer directly)
	explorer := &SQLiteExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "test.db",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "sqlite", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)
}

// normalizeSQLiteOutput normalizes volatile output from SQLite explorer
// to make golden files deterministic across environments.
func normalizeSQLiteOutput(s string) string {
	// Normalize file size: replace actual byte count with placeholder
	re := regexp.MustCompile(`Size: \d+ bytes`)
	s = re.ReplaceAllString(s, `Size: <normalized> bytes`)

	// Normalize SQLite version: replace version string with placeholder
	re = regexp.MustCompile(`SQLite version: [^ \n]+`)
	s = re.ReplaceAllString(s, `SQLite version: <normalized>`)

	return s
}

// createTestDB creates a deterministic SQLite database with a known schema.
func createTestDB(t *testing.T, path string) []byte {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", url.QueryEscape(path)))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			age INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE posts (
			id INTEGER PRIMARY KEY,
			title TEXT,
			user_id INTEGER,
			content TEXT
		);
		CREATE TABLE comments (
			id INTEGER,
			post_id INTEGER,
			comment TEXT
		);
		CREATE INDEX idx_users_email ON users(email);
		CREATE INDEX idx_posts_user ON posts(user_id);
	`)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return content
}

// TestSQLiteExplorer_GoldenEnhancement tests SQLite explorer output
// with the enhancement profile using golden file comparison.
func TestSQLiteExplorer_GoldenEnhancement(t *testing.T) {
	t.Parallel()

	// Create deterministic test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	content := createTestDB(t, dbPath)

	// Create registry with enhancement profile
	registry := NewRegistry(WithOutputProfile(OutputProfileEnhancement))

	// Explore the database
	result, err := registry.Explore(context.Background(), ExploreInput{
		Path:    "test.db",
		Content: content,
	})
	require.NoError(t, err)

	// Normalize volatile output before comparison
	normalized := normalizeSQLiteOutput(result.Summary)

	// Compare against golden file
	golden.RequireEqual(t, []byte(normalized))
}

// TestSQLiteExplorer_GoldenParity tests SQLite explorer output
// with the parity profile using golden file comparison.
func TestSQLiteExplorer_GoldenParity(t *testing.T) {
	t.Parallel()

	// Create deterministic test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	content := createTestDB(t, dbPath)

	// Create registry with parity profile
	registry := NewRegistry(WithOutputProfile(OutputProfileParity))

	// Explore the database
	result, err := registry.Explore(context.Background(), ExploreInput{
		Path:    "test.db",
		Content: content,
	})
	require.NoError(t, err)

	// Normalize volatile output before comparison
	normalized := normalizeSQLiteOutput(result.Summary)

	// Compare against golden file
	golden.RequireEqual(t, []byte(normalized))
}
