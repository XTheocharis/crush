package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestClassifyComplexity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		messages  []message.Message
		wantLevel ComplexityLevel
	}{
		{
			name:      "empty conversation is simple",
			messages:  nil,
			wantLevel: ComplexitySimple,
		},
		{
			name: "single file edit is simple",
			messages: []message.Message{
				{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: "Fix the typo in README.md"},
					},
				},
				{
					Role: message.Assistant,
					Parts: []message.ContentPart{
						message.TextContent{Text: "I'll fix that typo."},
						message.ToolCall{Name: "edit", Input: `{"file":"README.md"}`},
					},
				},
			},
			wantLevel: ComplexitySimple,
		},
		{
			name: "moderate tool usage is medium",
			messages: []message.Message{
				{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: "Update the config files"},
					},
				},
				{
					Role: message.Assistant,
					Parts: []message.ContentPart{
						message.ToolCall{Name: "view", Input: `{"file":"config.go"}`},
					},
				},
				{
					Role: message.Tool,
					Parts: []message.ContentPart{
						message.ToolResult{Content: "file contents"},
					},
				},
				{
					Role: message.Assistant,
					Parts: []message.ContentPart{
						message.ToolCall{Name: "edit", Input: `{"file":"config.go"}`},
					},
				},
				{
					Role: message.Tool,
					Parts: []message.ContentPart{
						message.ToolResult{Content: "ok"},
					},
				},
				{
					Role: message.Assistant,
					Parts: []message.ContentPart{
						message.ToolCall{Name: "edit", Input: `{"file":"config_test.go"}`},
					},
				},
				{
					Role: message.Tool,
					Parts: []message.ContentPart{
						message.ToolResult{Content: "ok"},
					},
				},
			},
			wantLevel: ComplexityMedium,
		},
		{
			name: "planning tools make it complex",
			messages: []message.Message{
				{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: "Plan the new auth system"},
					},
				},
				{
					Role: message.Assistant,
					Parts: []message.ContentPart{
						message.ToolCall{Name: "architect_plan", Input: `{}`},
					},
				},
			},
			wantLevel: ComplexityComplex,
		},
		{
			name: "many tool calls is complex",
			messages: func() []message.Message {
				var msgs []message.Message
				msgs = append(msgs, message.Message{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: "Refactor everything"},
					},
				})
				tools := []string{"view", "edit", "grep", "bash", "view", "edit", "bash"}
				for _, tool := range tools {
					msgs = append(msgs, message.Message{
						Role: message.Assistant,
						Parts: []message.ContentPart{
							message.ToolCall{Name: tool, Input: `{}`},
						},
					})
				}
				return msgs
			}(),
			wantLevel: ComplexityComplex,
		},
		{
			name: "large token count alone is medium",
			messages: []message.Message{
				{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: string(make([]byte, 80000))},
					},
				},
			},
			wantLevel: ComplexityMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyComplexity(tt.messages)
			require.Equal(t, tt.wantLevel, got)
		})
	}
}

func TestClassifyComplexityFromSignals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		signals   ComplexitySignals
		wantLevel ComplexityLevel
	}{
		{
			name:      "zero signals is simple",
			signals:   ComplexitySignals{},
			wantLevel: ComplexitySimple,
		},
		{
			name: "few tool calls and small tokens is simple",
			signals: ComplexitySignals{
				ToolCallCount:     1,
				TokenCount:        500,
				DistinctToolCount: 1,
			},
			wantLevel: ComplexitySimple,
		},
		{
			name: "3 tool calls crosses to medium",
			signals: ComplexitySignals{
				ToolCallCount:     3,
				TokenCount:        1000,
				DistinctToolCount: 2,
			},
			wantLevel: ComplexityMedium,
		},
		{
			name: "planning tools with small tokens is complex",
			signals: ComplexitySignals{
				ToolCallCount:     1,
				TokenCount:        500,
				HasPlanningTools:  true,
				DistinctToolCount: 3,
			},
			wantLevel: ComplexityComplex,
		},
		{
			name: "7+ tool calls is complex",
			signals: ComplexitySignals{
				ToolCallCount:     7,
				TokenCount:        1000,
				DistinctToolCount: 3,
			},
			wantLevel: ComplexityComplex,
		},
		{
			name: "medium tokens alone is medium",
			signals: ComplexitySignals{
				TokenCount: 8000,
			},
			wantLevel: ComplexityMedium,
		},
		{
			name: "large tokens with many distinct tools is complex",
			signals: ComplexitySignals{
				ToolCallCount:     3,
				TokenCount:        20000,
				DistinctToolCount: 5,
			},
			wantLevel: ComplexityComplex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyComplexityFromSignals(tt.signals)
			require.Equal(t, tt.wantLevel, got)
		})
	}
}

func TestComplexityLevelString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "simple", ComplexitySimple.String())
	require.Equal(t, "medium", ComplexityMedium.String())
	require.Equal(t, "complex", ComplexityComplex.String())
	require.Equal(t, "unknown", ComplexityLevel(99).String())
}

func TestComplexityLevelNumericPriority(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.15, ComplexitySimple.NumericPriority(), 0.001)
	require.InDelta(t, 0.4, ComplexityMedium.NumericPriority(), 0.001)
	require.InDelta(t, 0.7, ComplexityComplex.NumericPriority(), 0.001)
	require.InDelta(t, 0.15, ComplexityLevel(99).NumericPriority(), 0.001)
}

func TestIsPlanningTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"architect_plan", "architect_plan", true},
		{"operator", "operator_run", true},
		{"parallel", "parallel_execute", true},
		{"swarm", "swarm_dispatch", true},
		{"team_create", "team_create", true},
		{"plan prefix", "plan_step", true},
		{"case insensitive", "Architect_Plan", true},
		{"edit is not planning", "edit", false},
		{"view is not planning", "view", false},
		{"bash is not planning", "bash", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPlanningTool(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveWithComplexity(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	tests := []struct {
		name       string
		tokenCount int
		complexity ComplexityLevel
		want       config.SelectedModelType
	}{
		{
			name:       "simple under threshold stays small",
			tokenCount: 1000,
			complexity: ComplexitySimple,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "simple at threshold stays small",
			tokenCount: 4000,
			complexity: ComplexitySimple,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "simple over threshold goes large",
			tokenCount: 4001,
			complexity: ComplexitySimple,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "medium boosts 2x so 2001 tokens hits 4002 -> large",
			tokenCount: 2001,
			complexity: ComplexityMedium,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "medium at 2000 stays small (2x=4000, within threshold)",
			tokenCount: 2000,
			complexity: ComplexityMedium,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "complex boosts 4x so 1001 hits 4004 -> large",
			tokenCount: 1001,
			complexity: ComplexityComplex,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "complex at 1000 stays small (4x=4000, within threshold)",
			tokenCount: 1000,
			complexity: ComplexityComplex,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "backward compat: zero complexity behaves like simple",
			tokenCount: 1000,
			complexity: ComplexitySimple,
			want:       config.SelectedModelTypeSmall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := router.ResolveWithComplexity(tt.tokenCount, tt.complexity)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveBackwardCompatibility(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	tokenCounts := []int{0, 1000, 4000, 4001, 100000}
	for _, tc := range tokenCounts {
		resolveResult := router.Resolve(tc)
		complexityResult := router.ResolveWithComplexity(tc, ComplexitySimple)
		require.Equal(t, resolveResult, complexityResult,
			"Resolve(%d) should equal ResolveWithComplexity(%d, Simple)", tc, tc)
	}
}

func TestRouteForAgentBackwardCompatibility(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	result := router.RouteForAgent("test-agent", 1000)
	require.Equal(t, config.SelectedModelTypeSmall, result)
}

func TestRouteForAgentWithAgentTiers(t *testing.T) {
	t.Parallel()

	router := NewTierRouterWithAgentTiers(
		[]config.RoutingTier{
			{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
		},
		map[string][]config.RoutingTier{
			"task": {
				{UpToTokens: 2000, ModelType: config.SelectedModelTypeSmall},
				{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
			},
		},
	)

	require.Equal(t, config.SelectedModelTypeSmall, router.RouteForAgent("task", 2000))
	require.Equal(t, config.SelectedModelTypeLarge, router.RouteForAgent("task", 2001))
	require.Equal(t, config.SelectedModelTypeSmall, router.RouteForAgent("other", 4000))
}

func TestEmptyRouterReturnsLarge(t *testing.T) {
	t.Parallel()

	router := NewTierRouter(nil)
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(1000))
	require.Equal(t, config.SelectedModelTypeLarge,
		router.ResolveWithComplexity(1000, ComplexityComplex))
}

func TestResolveWithPhase(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	tests := []struct {
		name       string
		tokenCount int
		phase      AgentPhase
		want       config.SelectedModelType
	}{
		{
			name:       "reviewing under threshold stays small",
			tokenCount: 1000,
			phase:      PhaseReviewing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "reviewing over threshold goes large",
			tokenCount: 4001,
			phase:      PhaseReviewing,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "planning boosts 3x so 1334 hits 4002 -> large",
			tokenCount: 1334,
			phase:      PhasePlanning,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "planning at 1333 stays small (3x=3999, within threshold)",
			tokenCount: 1333,
			phase:      PhasePlanning,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "editing halves so 8001 hits 4000 -> still small",
			tokenCount: 8000,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "editing halves so 8001 becomes 4000 -> small (<=threshold)",
			tokenCount: 8001,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "editing halves so 8002 becomes 4001 -> large",
			tokenCount: 8002,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "planning small token stays small even with 3x boost",
			tokenCount: 1000,
			phase:      PhasePlanning,
			want:       config.SelectedModelTypeSmall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := router.ResolveWithPhase(tt.tokenCount, tt.phase)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveWithComplexityAndPhase(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	tests := []struct {
		name       string
		tokenCount int
		complexity ComplexityLevel
		phase      AgentPhase
		want       config.SelectedModelType
	}{
		{
			name:       "simple editing halves to small",
			tokenCount: 6000,
			complexity: ComplexitySimple,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "complex planning 4x3=12x boost still under threshold",
			tokenCount: 333,
			complexity: ComplexityComplex,
			phase:      PhasePlanning,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "complex planning 4x3=12x boost pushes over",
			tokenCount: 334,
			complexity: ComplexityComplex,
			phase:      PhasePlanning,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "medium reviewing 2x boost",
			tokenCount: 2001,
			complexity: ComplexityMedium,
			phase:      PhaseReviewing,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "medium editing 2x then 0.5x = 1x net",
			tokenCount: 4000,
			complexity: ComplexityMedium,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "medium editing 2x then 0.5x = 1x net over",
			tokenCount: 4001,
			complexity: ComplexityMedium,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "simple reviewing is unchanged",
			tokenCount: 1000,
			complexity: ComplexitySimple,
			phase:      PhaseReviewing,
			want:       config.SelectedModelTypeSmall,
		},
		{
			name:       "complex editing 4x then 0.5x = 2x net",
			tokenCount: 2001,
			complexity: ComplexityComplex,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeLarge,
		},
		{
			name:       "complex editing 4x then 0.5x = 2x net under",
			tokenCount: 2000,
			complexity: ComplexityComplex,
			phase:      PhaseEditing,
			want:       config.SelectedModelTypeSmall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := router.ResolveWithComplexityAndPhase(
				tt.tokenCount, tt.complexity, tt.phase,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPhaseRoutingIntegration(t *testing.T) {
	t.Parallel()

	t.Run("ClassifyPhase drives routing for planning prompt", func(t *testing.T) {
		t.Parallel()

		router := NewTierRouter([]config.RoutingTier{
			{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
		})

		phase := ClassifyPhase("Plan the architecture for the new auth system")
		require.Equal(t, PhasePlanning, phase)

		result := router.ResolveWithPhase(1500, phase)
		require.Equal(t, config.SelectedModelTypeLarge, result,
			"planning phase should push 1500 tokens into large tier")
	})

	t.Run("ClassifyPhase drives routing for editing prompt", func(t *testing.T) {
		t.Parallel()

		router := NewTierRouter([]config.RoutingTier{
			{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
		})

		phase := ClassifyPhase("Fix the bug in the login handler")
		require.Equal(t, PhaseEditing, phase)

		result := router.ResolveWithPhase(6000, phase)
		require.Equal(t, config.SelectedModelTypeSmall, result,
			"editing phase should keep 6000 tokens in small tier")
	})

	t.Run("empty router returns large regardless of phase", func(t *testing.T) {
		t.Parallel()

		router := NewTierRouter(nil)
		require.Equal(t, config.SelectedModelTypeLarge,
			router.ResolveWithPhase(1000, PhasePlanning))
		require.Equal(t, config.SelectedModelTypeLarge,
			router.ResolveWithComplexityAndPhase(1000, ComplexityComplex, PhasePlanning))
	})
}
