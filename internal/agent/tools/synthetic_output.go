package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

const (
	SyntheticOutputToolName = "synthetic_output"
)

//go:embed synthetic_output.md
var syntheticOutputDescription []byte

type SyntheticOutputParams struct {
	Content string `json:"content" description:"The synthetic output content to generate"`
	Format  string `json:"format" description:"Output format: text, json, or markdown"`
}

type SyntheticOutputResponseMetadata struct {
	Format string `json:"format"`
	Length int    `json:"length"`
}

func NewSyntheticOutputTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SyntheticOutputToolName,
		string(syntheticOutputDescription),
		func(ctx context.Context, params SyntheticOutputParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Content == "" {
				return fantasy.NewTextErrorResponse("missing content"), nil
			}

			format := params.Format
			if format == "" {
				format = "text"
			}

			var output string
			switch format {
			case "json":
				output = fmt.Sprintf(`{"output": %q}`, params.Content)
			case "markdown":
				output = fmt.Sprintf("```\n%s\n```", params.Content)
			case "text":
				output = params.Content
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unsupported format %q: use text, json, or markdown", format)), nil
			}

			metadata := SyntheticOutputResponseMetadata{
				Format: format,
				Length: len(output),
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(output), metadata), nil
		},
	)
}
