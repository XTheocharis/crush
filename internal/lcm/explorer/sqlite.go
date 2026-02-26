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
type SQLiteExplorer struct {
	formatterProfile OutputProfile
}

const (
	sqliteMagicHeader = "SQLite format 3\000"
	maxSampleRows     = 3
	maxCellLength     = 100
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

	// PARITY MODE: Sample row summaries
	sampleRows, err := e.getSampleRows(ctx, db, tables)
	if err == nil {
		summary.WriteString("\nSample row summaries:\n")
		for _, table := range tables {
			if rows, ok := sampleRows[table]; ok && len(rows) > 0 {
				fmt.Fprintf(&summary, "  %s:\n", table)
				for _, row := range rows {
					fmt.Fprintf(&summary, "    %s\n", row)
				}
			}
		}
	}

	// EXCEED MODE: Views, triggers, and constraints
	if e.formatterProfile == OutputProfileEnhancement {
		// Get view inventory
		views, err := e.getViews(ctx, db)
		if err == nil {
			fmt.Fprintf(&summary, "\nViews: %d\n", len(views))
			if len(views) > 0 {
				summary.WriteString("\nView inventory:\n")
				for _, view := range views {
					fmt.Fprintf(&summary, "  - %s\n", view.Name)
					fmt.Fprintf(&summary, "    SQL: %s\n", view.SQL)
				}
			}
		}

		// Get trigger inventory
		triggers, err := e.getTriggers(ctx, db)
		if err == nil {
			fmt.Fprintf(&summary, "\nTriggers: %d\n", len(triggers))
			if len(triggers) > 0 {
				summary.WriteString("\nTrigger inventory:\n")
				for _, trig := range triggers {
					fmt.Fprintf(&summary, "  - %s on %s (%s)\n", trig.Name, trig.Table, trig.Timing)
					fmt.Fprintf(&summary, "    Event: %s\n", trig.Event)
					fmt.Fprintf(&summary, "    SQL: %s\n", trig.SQL)
				}
			}
		}

		// Get constraint details
		constraints, err := e.getConstraints(ctx, db, tables)
		if err == nil {
			constraintCount := 0
			for _, tblConstraints := range constraints {
				constraintCount += len(tblConstraints)
			}
			if constraintCount > 0 {
				fmt.Fprintf(&summary, "\nConstraints: %d\n", constraintCount)
				summary.WriteString("\nConstraint details:\n")
				for _, table := range tables {
					if tblConstraints, ok := constraints[table]; ok && len(tblConstraints) > 0 {
						fmt.Fprintf(&summary, "  %s:\n", table)
						for _, c := range tblConstraints {
							fmt.Fprintf(&summary, "    - %s: %s\n", c.Type, c.Definition)
						}
					}
				}
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

type viewInfo struct {
	Name string
	SQL  string
}

type triggerInfo struct {
	Name   string
	Table  string
	Timing string // BEFORE, AFTER, INSTEAD OF
	Event  string // INSERT, UPDATE, DELETE
	SQL    string
}

type constraintInfo struct {
	Type       string // PRIMARY KEY, FOREIGN KEY, UNIQUE, CHECK
	Definition string
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

// getSampleRows gets sample rows from each table for parity mode.
func (e *SQLiteExplorer) getSampleRows(ctx context.Context, db *sql.DB, tables []string) (map[string][]string, error) {
	result := make(map[string][]string)

	for _, table := range tables {
		// Check row count first.
		var count int
		err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(table))).Scan(&count)
		if err != nil {
			continue
		}

		if count == 0 {
			result[table] = []string{"(empty table)"}
			continue
		}

		tableRows, err := e.getSampleRowsForTable(ctx, db, table)
		if err != nil {
			continue
		}

		// Add overflow marker if truncated.
		if count > maxSampleRows && len(tableRows) > 0 {
			overflow := overflowMarker(e.formatterProfile, count-maxSampleRows, false)
			tableRows = append(tableRows, overflow)
		}

		if len(tableRows) > 0 {
			result[table] = tableRows
		}
	}

	return result, nil
}

func (e *SQLiteExplorer) getSampleRowsForTable(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdentifier(table), maxSampleRows)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var tableRows []string
	for rows.Next() {
		values := make([]any, len(columns))
		valuesPtr := make([]any, len(columns))
		for i := range values {
			valuesPtr[i] = &values[i]
		}

		if err := rows.Scan(valuesPtr...); err != nil {
			continue
		}

		var cells []string
		for i, col := range columns {
			var val string
			if values[i] == nil {
				val = "NULL"
			} else {
				// Handle different types.
				switch v := values[i].(type) {
				case []byte:
					// Check if it's BLOB or text.
					if looksLikeBLOB(v) {
						val = fmt.Sprintf("<BLOB %d bytes>", len(v))
					} else {
						val = string(v)
						if len(val) > maxCellLength {
							val = val[:maxCellLength] + "..."
						}
					}
				default:
					val = fmt.Sprintf("%v", v)
					if len(val) > maxCellLength {
						val = val[:maxCellLength] + "..."
					}
				}
			}
			cells = append(cells, fmt.Sprintf("%s: %s", col, val))
		}
		tableRows = append(tableRows, "{ "+strings.Join(cells, ", ")+" }")
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tableRows, nil
}

// getViews gets view definitions for exceed mode.
func (e *SQLiteExplorer) getViews(ctx context.Context, db *sql.DB) ([]viewInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name, sql FROM sqlite_master WHERE type='view' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []viewInfo
	for rows.Next() {
		var name, sql sql.NullString
		if err := rows.Scan(&name, &sql); err != nil {
			continue
		}
		viewSQL := sql.String
		if viewSQL == "" {
			viewSQL = "(SQL not available)"
		}
		views = append(views, viewInfo{Name: name.String, SQL: viewSQL})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return views, nil
}

// getTriggers gets trigger definitions for exceed mode.
func (e *SQLiteExplorer) getTriggers(ctx context.Context, db *sql.DB) ([]triggerInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name, tbl_name, sql FROM sqlite_master WHERE type='trigger' ORDER BY tbl_name, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []triggerInfo
	for rows.Next() {
		var name, table, sql sql.NullString
		if err := rows.Scan(&name, &table, &sql); err != nil {
			continue
		}

		sqlStr := sql.String
		if sqlStr == "" {
			sqlStr = "(SQL not available)"
		}

		// Parse timing and event from SQL (pattern: BEFORE/AFTER/INSTEAD OF INSERT/UPDATE/DELETE ON ...)
		timing := "UNKNOWN"
		event := "UNKNOWN"
		sqlUpper := strings.ToUpper(sqlStr)

		switch {
		case strings.HasPrefix(sqlUpper, "BEFORE "):
			timing = "BEFORE"
		case strings.HasPrefix(sqlUpper, "AFTER "):
			timing = "AFTER"
		case strings.HasPrefix(sqlUpper, "INSTEAD OF "):
			timing = "INSTEAD OF"
		}

		if strings.Contains(sqlUpper, "INSERT") {
			event = "INSERT"
		} else if strings.Contains(sqlUpper, "UPDATE") {
			event = "UPDATE"
		} else if strings.Contains(sqlUpper, "DELETE") {
			event = "DELETE"
		}

		triggers = append(triggers, triggerInfo{
			Name:   name.String,
			Table:  table.String,
			Timing: timing,
			Event:  event,
			SQL:    sqlStr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return triggers, nil
}

// getConstraints gets constraint details for exceed mode.
func (e *SQLiteExplorer) getConstraints(ctx context.Context, db *sql.DB, tables []string) (map[string][]constraintInfo, error) {
	result := make(map[string][]constraintInfo)

	for _, table := range tables {
		var constraints []constraintInfo

		// Get foreign key info via PRAGMA.
		fkQuery := fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdentifier(table))
		fkRows, err := db.QueryContext(ctx, fkQuery)
		if err == nil {
			func() {
				defer fkRows.Close()
				for fkRows.Next() {
					var id, seq int
					var tableStr, from, to, onUpdate, onDelete, match sql.NullString
					if err := fkRows.Scan(&id, &seq, &tableStr, &from, &to, &onUpdate, &onDelete, &match); err != nil {
						continue
					}
					def := fmt.Sprintf("%s.%s -> %s.%s", table, from.String, tableStr.String, to.String)
					constraints = append(constraints, constraintInfo{
						Type:       "FOREIGN KEY",
						Definition: def,
					})
				}
				if err := fkRows.Err(); err != nil {
					return
				}
			}()
		}

		// Get UNIQUE constraints from PRAGMA index_list.
		idxQuery := fmt.Sprintf("PRAGMA index_list(%s)", quoteIdentifier(table))
		idxRows, err := db.QueryContext(ctx, idxQuery)
		if err == nil {
			func() {
				defer idxRows.Close()
				for idxRows.Next() {
					var seq int
					var name, origin, partial sql.NullString
					var unique int
					if err := idxRows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
						continue
					}
					if unique > 0 && origin.String != "pk" {
						constraints = append(constraints, constraintInfo{
							Type:       "UNIQUE INDEX",
							Definition: name.String,
						})
					}
				}
				if err := idxRows.Err(); err != nil {
					return
				}
			}()
		}

		// Get CHECK constraints from CREATE TABLE SQL
		var createSQL string
		err = db.QueryRowContext(ctx,
			"SELECT sql FROM sqlite_master WHERE type='table' AND name=?",
			table).Scan(&createSQL)
		if err == nil {
			// Parse CHECK constraints from CREATE TABLE SQL
			checks := parseCheckConstraints(createSQL)
			for _, check := range checks {
				constraints = append(constraints, constraintInfo{
					Type:       "CHECK",
					Definition: check,
				})
			}
		}

		if len(constraints) > 0 {
			result[table] = constraints
		}
	}

	return result, nil
}

// parseCheckConstraints extracts CHECK constraints from CREATE TABLE SQL.
func parseCheckConstraints(createSQL string) []string {
	var checks []string
	// Find CHECK (...) patterns
	sqlLower := strings.ToLower(createSQL)
	i := 0
	for i < len(createSQL) {
		pos := strings.Index(sqlLower[i:], "check")
		if pos == -1 {
			break
		}
		pos += i
		// Skip if it's not followed by (
		if pos+5 >= len(createSQL) || createSQL[pos+5] != '(' {
			i = pos + 5
			continue
		}
		// Find matching closing parenthesis
		start := pos + 6
		depth := 1
		j := start
		for j < len(createSQL) && depth > 0 {
			switch createSQL[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth == 0 {
			checks = append(checks, strings.TrimSpace(createSQL[start:j-1]))
		}
		i = j
	}
	return checks
}

// looksLikeBLOB heuristically determines if bytes represent binary data.
func looksLikeBLOB(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Check for non-printable bytes
	for _, b := range data {
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			return true
		}
		if b > 126 {
			return true
		}
	}
	return false
}

// quoteIdentifier quotes a SQLite identifier (table or column name).
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
