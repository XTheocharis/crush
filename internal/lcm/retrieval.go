package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Bindle retrieves a compressed summary by ID and returns formatted text.
// Returns a human-readable "not found" message when the summary does not exist.
func (s *Store) Bindle(ctx context.Context, summaryID string) (string, error) {
	row, err := s.q.GetLcmSummary(ctx, summaryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("Summary not found: %s", summaryID), nil
		}
		return "", fmt.Errorf("querying summary %s: %v: %w", summaryID, ErrStorageQuery, err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Summary ID: %s\n", row.SummaryID)
	fmt.Fprintf(&sb, "Session: %s\n", row.SessionID)
	fmt.Fprintf(&sb, "Kind: %s\n", row.Kind)
	fmt.Fprintf(&sb, "Tokens: %d\n", row.TokenCount)
	fmt.Fprintf(&sb, "Created: %d\n", row.CreatedAt)

	if row.FileIds != "" && row.FileIds != "[]" {
		fmt.Fprintf(&sb, "File IDs: %s\n", row.FileIds)
	}

	// Load parent summaries for condensed summaries.
	if row.Kind == KindCondensed {
		parents, err := s.q.ListLcmSummaryParents(ctx, summaryID)
		if err != nil {
			return "", fmt.Errorf("listing parents for %s: %w", summaryID, err)
		}
		if len(parents) > 0 {
			ids := make([]string, len(parents))
			for i, p := range parents {
				ids[i] = p.ParentSummaryID
			}
			fmt.Fprintf(&sb, "Parents: %s\n", strings.Join(ids, ", "))
		}
	}

	fmt.Fprintf(&sb, "\nContent:\n%s\n", row.Content)
	return sb.String(), nil
}

// Ancestry walks the parent chain of a summary using a recursive CTE and
// returns a formatted ancestry trace. Returns a "not found" message when the
// summary does not exist or has no ancestors.
func (s *Store) Ancestry(ctx context.Context, summaryID string) (string, error) {
	// Verify the summary exists first.
	_, err := s.q.GetLcmSummary(ctx, summaryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("Summary not found: %s", summaryID), nil
		}
		return "", fmt.Errorf("checking summary %s: %v: %w", summaryID, ErrStorageQuery, err)
	}

	query := `
		WITH RECURSIVE chain(summary_id, depth) AS (
			SELECT ?, 0
			UNION
			SELECT sp.parent_summary_id, c.depth + 1
			FROM lcm_summary_parents sp
			JOIN chain c ON sp.summary_id = c.summary_id
		)
		SELECT c.summary_id, ls.kind, ls.token_count,
		       SUBSTR(ls.content, 1, 120) AS preview
		FROM chain c
		JOIN lcm_summaries ls ON ls.summary_id = c.summary_id
		ORDER BY c.depth ASC`

	rows, err := s.rawDB.QueryContext(ctx, query, summaryID)
	if err != nil {
		return "", fmt.Errorf("walking ancestry for %s: %w", summaryID, err)
	}
	defer rows.Close()

	type ancestor struct {
		summaryID string
		kind      string
		tokens    int64
		preview   string
	}

	var ancestors []ancestor
	for rows.Next() {
		var a ancestor
		if err := rows.Scan(&a.summaryID, &a.kind, &a.tokens, &a.preview); err != nil {
			return "", fmt.Errorf("scanning ancestor: %w", err)
		}
		ancestors = append(ancestors, a)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating ancestors: %w", err)
	}

	if len(ancestors) == 0 {
		return fmt.Sprintf("Summary not found: %s", summaryID), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Ancestry chain for %s (%d levels):\n\n", summaryID, len(ancestors))
	for i, a := range ancestors {
		fmt.Fprintf(&sb, "  [%d] %s  (kind=%s, tokens=%d)\n", i, a.summaryID, a.kind, a.tokens)
		fmt.Fprintf(&sb, "      %s\n", a.preview)
	}
	return sb.String(), nil
}

// Dolt retrieves all summaries for a session and returns them formatted.
// Returns a message indicating no summaries were found when the session has
// none.
func (s *Store) Dolt(ctx context.Context, sessionID string) (string, error) {
	summaries, err := s.q.ListLcmSummariesBySession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("listing summaries for session %s: %v: %w", sessionID, ErrStorageQuery, err)
	}

	if len(summaries) == 0 {
		return fmt.Sprintf("No summaries found for session: %s", sessionID), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Summaries for session %s (%d total):\n\n", sessionID, len(summaries))
	for i, s := range summaries {
		preview := s.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Fprintf(&sb, "  [%d] %s  (kind=%s, tokens=%d)\n", i, s.SummaryID, s.Kind, s.TokenCount)
		fmt.Fprintf(&sb, "      %s\n", preview)
	}
	return sb.String(), nil
}

// QueryByLineage traverses the summary DAG from a starting summary in the
// requested direction, up to maxDepth levels. It uses recursive CTEs on
// lcm_summary_parents and returns structured LineageNode values.
func (s *Store) QueryByLineage(ctx context.Context, sessionID, summaryID string, direction LineageDirection, maxDepth int) ([]LineageNode, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	var nodes []LineageNode

	if direction == LineageAncestors || direction == LineageBoth {
		anc, err := s.queryLineageDirection(ctx, sessionID, summaryID, maxDepth, true)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, anc...)
	}

	if direction == LineageDescendants || direction == LineageBoth {
		desc, err := s.queryLineageDirection(ctx, sessionID, summaryID, maxDepth, false)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, desc...)
	}

	return nodes, nil
}

func (s *Store) queryLineageDirection(ctx context.Context, sessionID, startSummaryID string, maxDepth int, ancestors bool) ([]LineageNode, error) {
	var cte string
	if ancestors {
		cte = `
			WITH RECURSIVE lineage(summary_id, parent_id, depth) AS (
				SELECT ?, '', 0
				UNION
				SELECT sp.parent_summary_id, sp.summary_id, l.depth + 1
				FROM lcm_summary_parents sp
				JOIN lineage l ON sp.summary_id = l.summary_id
				WHERE l.depth < ?
			)`
	} else {
		cte = `
			WITH RECURSIVE lineage(summary_id, parent_id, depth) AS (
				SELECT ?, '', 0
				UNION
				SELECT sp.summary_id, sp.parent_summary_id, l.depth + 1
				FROM lcm_summary_parents sp
				JOIN lineage l ON sp.parent_summary_id = l.summary_id
				WHERE l.depth < ?
			)`
	}

	query := cte + `
		SELECT l.summary_id, l.parent_id, l.depth, ls.token_count, ls.kind
		FROM lineage l
		JOIN lcm_summaries ls ON ls.summary_id = l.summary_id
		WHERE ls.session_id = ?
		ORDER BY l.depth ASC`

	rows, err := s.rawDB.QueryContext(ctx, query, startSummaryID, maxDepth, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying lineage for %s: %w", startSummaryID, err)
	}
	defer rows.Close()

	var nodes []LineageNode
	for rows.Next() {
		var n LineageNode
		if err := rows.Scan(&n.SummaryID, &n.ParentID, &n.Depth, &n.TokenCount, &n.Kind); err != nil {
			return nil, fmt.Errorf("scanning lineage node: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating lineage nodes: %w", err)
	}
	return nodes, nil
}

// Lineage retrieves the lineage of a summary and returns formatted text.
func (s *Store) Lineage(ctx context.Context, sessionID, summaryID string, direction LineageDirection, maxDepth int) (string, error) {
	nodes, err := s.QueryByLineage(ctx, sessionID, summaryID, direction, maxDepth)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return fmt.Sprintf("No lineage found for summary %s in session %s", summaryID, sessionID), nil
	}

	dirLabel := "both"
	switch direction {
	case LineageAncestors:
		dirLabel = "ancestors"
	case LineageDescendants:
		dirLabel = "descendants"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Lineage for %s (direction=%s, max_depth=%d, %d nodes):\n\n", summaryID, dirLabel, maxDepth, len(nodes))
	for _, n := range nodes {
		indent := strings.Repeat("  ", n.Depth)
		parentInfo := ""
		if n.ParentID != "" {
			parentInfo = fmt.Sprintf(" parent=%s", n.ParentID)
		}
		fmt.Fprintf(&sb, "%sdepth=%d  %s  (kind=%s, tokens=%d%s)\n", indent, n.Depth, n.SummaryID, n.Kind, n.TokenCount, parentInfo)
	}
	return sb.String(), nil
}

// Archive searches summary content using FTS5 and returns formatted matches.
// Returns a "no matches" message when nothing is found.
func (s *Store) Archive(ctx context.Context, sessionID, pattern string) (string, error) {
	results, err := s.SearchSummaries(ctx, sessionID, pattern)
	if err != nil {
		return "", fmt.Errorf("searching summaries: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No summaries matching %q found in session: %s", pattern, sessionID), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d summaries matching %q in session %s:\n\n", len(results), pattern, sessionID)
	for i, r := range results {
		fmt.Fprintf(&sb, "  [%d] %s  (kind=%s)\n", i, r.SummaryID, r.Kind)
	}
	return sb.String(), nil
}

// Sprig retrieves the most recent summary for a session and returns formatted
// text. Returns a "not found" message when the session has no summaries.
func (s *Store) Sprig(ctx context.Context, sessionID string) (string, error) {
	query := `
		SELECT summary_id, kind, content, token_count, file_ids, created_at
		FROM lcm_summaries
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT 1`

	var summaryID, kind, content, fileIDs string
	var tokenCount, createdAt int64

	err := s.rawDB.QueryRowContext(ctx, query, sessionID).Scan(
		&summaryID, &kind, &content, &tokenCount, &fileIDs, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("No summaries found for session: %s", sessionID), nil
		}
		return "", fmt.Errorf("querying latest summary for session %s: %v: %w", sessionID, ErrStorageQuery, err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Latest summary for session %s:\n\n", sessionID)
	fmt.Fprintf(&sb, "Summary ID: %s\n", summaryID)
	fmt.Fprintf(&sb, "Kind: %s\n", kind)
	fmt.Fprintf(&sb, "Tokens: %d\n", tokenCount)
	fmt.Fprintf(&sb, "Created: %d\n", createdAt)

	if fileIDs != "" && fileIDs != "[]" {
		fmt.Fprintf(&sb, "File IDs: %s\n", fileIDs)
	}

	fmt.Fprintf(&sb, "\nContent:\n%s\n", content)
	return sb.String(), nil
}
