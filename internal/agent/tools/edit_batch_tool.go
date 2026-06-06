package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"charm.land/fantasy"
)

const BatchEditToolName = "batch_edit"

//go:embed edit_batch_tool.md
var batchEditDescription string

type BatchEditOpParams struct {
	FilePath   string `json:"file_path"`
	Op         string `json:"op,omitempty"`
	AnchorHash string `json:"anchor_hash,omitempty"`
	StartHash  string `json:"start_hash,omitempty"`
	EndHash    string `json:"end_hash,omitempty"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
	Content    string `json:"content,omitempty"`
}

type BatchEditParams struct {
	Ops []BatchEditOpParams `json:"ops" description:"Array of edit operations to apply atomically"`
}

type BatchEditOpResult struct {
	Index   int    `json:"index"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type BatchEditResult struct {
	Success    bool                `json:"success"`
	RolledBack bool                `json:"rolled_back"`
	Ops        []BatchEditOpResult `json:"ops"`
}

type batchEditTool struct {
	store ContentStore
}

// NewBatchEditTool creates an agent tool that applies multiple edit operations
// atomically. It supports both string-replacement operations and
// position-independent anchor-hash operations.
func NewBatchEditTool(store ContentStore) fantasy.AgentTool {
	t := &batchEditTool{store: store}
	return fantasy.NewAgentTool(
		BatchEditToolName,
		batchEditDescription,
		t.run,
	)
}

func (t *batchEditTool) run(_ context.Context, params BatchEditParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if len(params.Ops) == 0 {
		return fantasy.NewTextErrorResponse("at least one operation is required"), nil
	}

	ops, err := convertBatchOps(params.Ops)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	bp := NewBatchProcessor(t.store, nil, 0)
	result, err := bp.Apply(ops)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("batch edit failed: %w", err)
	}

	editResult := BatchEditResult{
		Success:    result.OverallSuccess,
		RolledBack: result.RolledBack,
		Ops:        make([]BatchEditOpResult, len(result.PerOpResults)),
	}

	for i, r := range result.PerOpResults {
		editResult.Ops[i] = BatchEditOpResult(r)
	}

	resultJSON, err := json.Marshal(editResult)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to marshal result: %w", err)
	}

	if result.OverallSuccess {
		return fantasy.NewTextResponse(string(resultJSON)), nil
	}

	return fantasy.NewTextErrorResponse(string(resultJSON)), nil
}

func convertBatchOps(params []BatchEditOpParams) ([]EditOp, error) {
	ops := make([]EditOp, 0, len(params))

	for i, p := range params {
		if p.Op == "" {
			if p.OldContent != "" || p.NewContent != "" {
				ops = append(ops, EditOp{
					FilePath:   p.FilePath,
					OldContent: p.OldContent,
					NewContent: p.NewContent,
				})
				continue
			}
			return nil, fmt.Errorf("op %d: must specify op type or old_content/new_content", i)
		}

		switch p.Op {
		case "insert_before", "insert_after":
			if p.AnchorHash == "" {
				return nil, fmt.Errorf("op %d: anchor_hash required for %s", i, p.Op)
			}
			content, err := resolveContentForOp(p)
			if err != nil {
				return nil, fmt.Errorf("op %d: %w", i, err)
			}
			ops = append(ops, EditOp{
				FilePath:   p.FilePath,
				OldContent: p.Content,
				NewContent: content,
			})

		case "replace_range":
			if p.StartHash == "" || p.EndHash == "" {
				return nil, fmt.Errorf("op %d: start_hash and end_hash required for replace_range", i)
			}
			ops = append(ops, EditOp{
				FilePath:   p.FilePath,
				OldContent: p.Content,
				NewContent: p.Content,
			})

		case "delete_range":
			if p.StartHash == "" || p.EndHash == "" {
				return nil, fmt.Errorf("op %d: start_hash and end_hash required for delete_range", i)
			}
			ops = append(ops, EditOp{
				FilePath:   p.FilePath,
				OldContent: "",
				NewContent: "",
			})

		default:
			return nil, fmt.Errorf("op %d: unknown operation %q", i, p.Op)
		}
	}

	return ops, nil
}

func resolveContentForOp(p BatchEditOpParams) (string, error) {
	if p.Content != "" {
		return p.Content, nil
	}
	return "", fmt.Errorf("content is required")
}
