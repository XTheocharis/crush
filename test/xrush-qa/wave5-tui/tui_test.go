package wave5_test

import (
	"testing"

	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/charmbracelet/crush/internal/ui/diffview"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

// TestDiffViewRenders verifies that a DiffView with before/after content
// produces a non-empty unified diff output without panicking. It uses
// golden-file snapshot testing to lock the render output.
func TestDiffViewRenders(t *testing.T) {
	t.Parallel()

	before := `package main

func main() {
	fmt.Println("hello")
}
`
	after := `package main

import "fmt"

func main() {
	fmt.Println("hello world")
}
`

	dv := diffview.New().
		Before("main.go", before).
		After("main.go", after)

	output := dv.Unified().String()
	require.NotEmpty(t, output, "DiffView must produce non-empty output")
	golden.RequireEqual(t, []byte(output))
}

// TestDiffViewSplitRenders verifies that a split-layout DiffView renders
// side-by-side output without error.
func TestDiffViewSplitRenders(t *testing.T) {
	t.Parallel()

	before := "line1\nline2\n"
	after := "line1\nmodified\n"

	dv := diffview.New().
		Before("file.txt", before).
		After("file.txt", after).
		Width(80)

	output := dv.Split().String()
	require.NotEmpty(t, output, "Split DiffView must produce non-empty output")
	golden.RequireEqual(t, []byte(output))
}

// TestCompactionPillStatus verifies that session.HasIncompleteTodos correctly
// reports the status of todo items, which directly drives the compaction pill
// visibility in the TUI pills panel.
func TestCompactionPillStatus(t *testing.T) {
	t.Parallel()

	t.Run("empty todos are not incomplete", func(t *testing.T) {
		t.Parallel()
		require.False(t, session.HasIncompleteTodos(nil))
		require.False(t, session.HasIncompleteTodos([]session.Todo{}))
	})

	t.Run("all completed todos are not incomplete", func(t *testing.T) {
		t.Parallel()
		todos := []session.Todo{
			{Status: session.TodoStatusCompleted, Content: "done"},
		}
		require.False(t, session.HasIncompleteTodos(todos))
	})

	t.Run("pending todo is incomplete", func(t *testing.T) {
		t.Parallel()
		todos := []session.Todo{
			{Status: session.TodoStatusPending, Content: "waiting"},
		}
		require.True(t, session.HasIncompleteTodos(todos))
	})

	t.Run("in-progress todo is incomplete", func(t *testing.T) {
		t.Parallel()
		todos := []session.Todo{
			{Status: session.TodoStatusInProgress, Content: "active"},
		}
		require.True(t, session.HasIncompleteTodos(todos))
	})

	t.Run("mixed statuses with one incomplete", func(t *testing.T) {
		t.Parallel()
		todos := []session.Todo{
			{Status: session.TodoStatusCompleted, Content: "done"},
			{Status: session.TodoStatusInProgress, Content: "active"},
			{Status: session.TodoStatusPending, Content: "waiting"},
		}
		require.True(t, session.HasIncompleteTodos(todos))
	})
}

// TestRewindActionMenuOptions verifies that the rewind action types carry the
// correct RewindMode values matching the three rewind strategies exposed in
// the message options menu.
func TestRewindActionMenuOptions(t *testing.T) {
	t.Parallel()

	t.Run("code-only mode is defined", func(t *testing.T) {
		t.Parallel()
		action := dialog.ActionRewind{SessionID: "s1", Seq: 5, Mode: rewind.RewindCodeOnly}
		require.Equal(t, "s1", action.SessionID)
		require.Equal(t, 5, action.Seq)
		require.Equal(t, rewind.RewindCodeOnly, action.Mode)
	})

	t.Run("convo-only mode is defined", func(t *testing.T) {
		t.Parallel()
		action := dialog.ActionRewind{SessionID: "s2", Seq: 3, Mode: rewind.RewindConvoOnly}
		require.Equal(t, rewind.RewindConvoOnly, action.Mode)
	})

	t.Run("both mode is defined", func(t *testing.T) {
		t.Parallel()
		action := dialog.ActionRewind{SessionID: "s3", Seq: 10, Mode: rewind.RewindBoth}
		require.Equal(t, rewind.RewindBoth, action.Mode)
	})

	t.Run("modes are distinct", func(t *testing.T) {
		t.Parallel()
		modes := []rewind.RewindMode{
			rewind.RewindCodeOnly,
			rewind.RewindConvoOnly,
			rewind.RewindBoth,
		}
		seen := map[rewind.RewindMode]bool{}
		for _, m := range modes {
			require.False(t, seen[m], "duplicate RewindMode: %d", m)
			seen[m] = true
		}
	})
}

// TestMessageActionsMenuPopulated verifies that the MessageOptions dialog
// satisfies the Dialog interface and has the expected ID after construction.
func TestMessageActionsMenuPopulated(t *testing.T) {
	t.Parallel()

	com := common.DefaultCommon(nil)
	mo := dialog.NewMessageOptions(com, "session-1", 7, "msg-abc")

	require.Equal(t, dialog.MessageOptionsID, mo.ID())

	var _ dialog.Dialog = mo
}

// TestRepoMapRefreshActionRegistered verifies that the ActionRefreshRepoMap
// type exists, carries the expected SessionID field, and is distinct from
// other dialog actions.
func TestRepoMapRefreshActionRegistered(t *testing.T) {
	t.Parallel()

	t.Run("carries session ID", func(t *testing.T) {
		t.Parallel()
		action := dialog.ActionRefreshRepoMap{SessionID: "sess-repo-refresh"}
		require.Equal(t, "sess-repo-refresh", action.SessionID)
	})

	t.Run("is distinct from other actions", func(t *testing.T) {
		t.Parallel()
		refresh := dialog.ActionRefreshRepoMap{SessionID: "s1"}
		summarize := dialog.ActionSummarize{SessionID: "s1"}
		require.NotEqual(t, refresh, summarize)
	})
}

// TestFilterableListBasics verifies the list.FilterableList construction and
// item management used by the message options and commands dialogs.
func TestFilterableListBasics(t *testing.T) {
	t.Parallel()

	fl := list.NewFilterableList()
	require.NotNil(t, fl)

	items := []list.FilterableItem{
		&testFilterableItem{label: "option-A"},
		&testFilterableItem{label: "option-B"},
		&testFilterableItem{label: "option-C"},
	}
	fl.SetItems(items...)

	filtered := fl.FilteredItems()
	require.Len(t, filtered, 3)
}

type testFilterableItem struct {
	*list.Versioned
	label string
}

func (t *testFilterableItem) Filter() string   { return t.label }
func (t *testFilterableItem) Finished() bool    { return true }
func (t *testFilterableItem) Render(int) string  { return t.label }
func (t *testFilterableItem) ID() string         { return t.label }

// TestDiffViewEmptyContent verifies that an empty before/after pair does not
// panic and produces deterministic output.
func TestDiffViewEmptyContent(t *testing.T) {
	t.Parallel()

	dv := diffview.New().
		Before("empty.txt", "").
		After("empty.txt", "")

	output := dv.Unified().String()
	require.Empty(t, output, "empty content produces no diff output")
}

// TestDiffViewNoChange verifies identical before/after content renders a diff
// with no hunks.
func TestDiffViewNoChange(t *testing.T) {
	t.Parallel()

	content := "unchanged line\n"
	dv := diffview.New().
		Before("same.txt", content).
		After("same.txt", content)

	output := dv.Unified().String()
	require.Empty(t, output, "identical content produces no diff output")
}

// TestTodoStatusConstants verifies that the todo status constants used to
// drive the pills panel UI have the expected string values.
func TestTodoStatusConstants(t *testing.T) {
	t.Parallel()

	require.Equal(t, session.TodoStatus("pending"), session.TodoStatusPending)
	require.Equal(t, session.TodoStatus("in_progress"), session.TodoStatusInProgress)
	require.Equal(t, session.TodoStatus("completed"), session.TodoStatusCompleted)
}
