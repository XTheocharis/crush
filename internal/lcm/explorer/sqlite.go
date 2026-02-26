package explorer

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLiteExplorer explores SQLite database files.
type SQLiteExplorer struct{}

const (
	sqliteMagicHeader = "SQLite format 3\000"
)

func (e *SQLiteExplorer) CanHandle(path string, content []byte) bool {
	// Check by extension first
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "sqlite" || ext == "sqlite3" || ext == "db" {
		return true
	}
	// Check magic header
	header := ""
	if len(content) >= len(sqliteMagicHeader) {
		header = string(content[:len(sqliteMagicHeader)])
	}
	return header == sqliteMagicHeader
}

func (e *SQLiteExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	var summary strings.Builder

	fmt.Fprintf(&summary, "SQLite database: %s\n", name)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Create temp file for SQLite library (it requires a file path, not bytes)
	tempFile, err := os.CreateTemp("", "crush-sqlite-*.db")
	if err != nil {
		summary.WriteString("\nError: Could not create temp file for SQLite inspection")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}
	tempPath := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempPath)
	}()

	// Write content to temp file
	if _, err := tempFile.Write(input.Content); err != nil {
		summary.WriteString("\nError: Could not write database content to temp file")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}
	tempFile.Close()

	// Open database in read-only mode
	dsn := fmt.Sprintf("file:%s?mode=ro", url.QueryEscape(tempPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		summary.WriteString("\nError: Could not open SQLite database (may be corrupted or invalid)")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}
	defer db.Close()

	// Verify it's a valid SQLite database by checking SQLite version
	var version string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		summary.WriteString("\nError: Invalid SQLite database file")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}
	fmt.Fprintf(&summary, "SQLite version: %s\n", version)

	// Get table inventory
	tables, err := e.getTables(ctx, db)
	if err != nil {
		summary.WriteString("\nError: Could not read table inventory")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	fmt.Fprintf(&summary, "Tables: %d\n", len(tables))
	if len(tables) > 0 {
		summary.WriteString("\nTable inventory:\n")
		for _, table := range tables {
			fmt.Fprintf(&summary, "  - %s\n", table)
		}
	}

	// Get index inventory
	indexes, err := e.getIndexes(ctx, db)
	if err != nil {
		summary.WriteString("\nError: Could not read index inventory")
		result := summary.String()
		return ExploreResult{
			Summary:       result,
			ExplorerUsed:  "sqlite",
			TokenEstimate: estimateTokens(result),
		}, nil
	}

	fmt.Fprintf(&summary, "\nIndexes: %d\n", len(indexes))
	if len(indexes) > 0 {
		summary.WriteString("\nIndex inventory:\n")
		for _, idx := range indexes {
			fmt.Fprintf(&summary, "  - %s on %s\n", idx.Name, idx.Table)
		}
	}

	// Get per-table column summaries
	if len(tables) > 0 {
		summary.WriteString("\nSchema details:\n")
		for _, table := range tables {
			columns, err := e.getColumns(ctx, db, table)
			if err != nil {
				fmt.Fprintf(&summary, "  %s: (error reading schema)\n", table)
				continue
			}
			if len(columns) == 0 {
				fmt.Fprintf(&summary, "  %s: (no columns)\n", table)
				continue
			}
			fmt.Fprintf(&summary, "  %s (%d columns):\n", table, len(columns))
			for _, col := range columns {
				fmt.Fprintf(&summary, "    - %s %s\n", col.Name, col.Type)
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "sqlite",
		TokenEstimate: estimateTokens(result),
	}, nil
}

type indexInfo struct {
	Name  string
	Table string
}

type columnInfo struct {
	Name string
	Type string
}

func (e *SQLiteExplorer) getTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func (e *SQLiteExplorer) getIndexes(ctx context.Context, db *sql.DB) ([]indexInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name, tbl_name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY tbl_name, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []indexInfo
	for rows.Next() {
		var idx indexInfo
		if err := rows.Scan(&idx.Name, &idx.Table); err != nil {
			continue
		}
		indexes = append(indexes, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return indexes, nil
}

func (e *SQLiteExplorer) getColumns(ctx context.Context, db *sql.DB, table string) ([]columnInfo, error) {
	// Use PRAGMA table_info to get column information
	query := fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []columnInfo
	for rows.Next() {
		var (
			cid       int
			name      string
			dataType  string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		typeStr := dataType
		if pk > 0 {
			typeStr += " (PK)"
		}
		if notnull > 0 {
			typeStr += " NOT NULL"
		}
		columns = append(columns, columnInfo{Name: name, Type: typeStr})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// quoteIdentifier quotes a SQLite identifier (table or column name).
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
