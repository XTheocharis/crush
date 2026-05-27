package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
)

const (
	defaultMaxDepth       = 3
	defaultMaxWorkers     = 16
	minTaskLength     int = 10
)

// DecomposeStrategy selects how an Operator decomposes a task into subtasks.
type DecomposeStrategy int

const (
	// StrategyLLMMap decomposes via a plan, then runs subtasks in parallel
	// with prompt-only context. Up to 16 workers.
	StrategyLLMMap DecomposeStrategy = iota

	// StrategyAgenticMap gives each subtask its own subagent with full
	// context, running in parallel.
	StrategyAgenticMap

	// StrategyBatch runs subtasks that share a mutable context object.
	StrategyBatch

	// StrategySequential runs subtasks one after another, piping each
	// output into the next input.
	StrategySequential
)

func (s DecomposeStrategy) String() string {
	switch s {
	case StrategyLLMMap:
		return "llm-map"
	case StrategyAgenticMap:
		return "agentic-map"
	case StrategyBatch:
		return "batch"
	case StrategySequential:
		return "sequential"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// Subtask is a single decomposed unit of work.
type Subtask struct {
	ID      string
	Task    string
	Context map[string]string
	Tools   []string
}

// Decomposer splits a parent task into subtasks. Implementations may use an
// LLM, rule-based heuristics, or return a fixed plan.
type Decomposer interface {
	Decompose(ctx context.Context, task string, context map[string]string) ([]Subtask, error)
}

// SubagentExecutor runs a single subtask. In production this wraps
// StructuredSubagent.Execute; in tests it can be replaced with a stub.
type SubagentExecutor func(ctx context.Context, req StructuredRequest) (StructuredResponse, error)

// OperatorResult holds the outcome of an operator run.
type OperatorResult struct {
	Success    bool
	Result     string
	SubResults []StructuredResponse
	Depth      int
	Strategy   DecomposeStrategy
	Error      string
}

// OperatorConfig controls operator behaviour.
type OperatorConfig struct {
	MaxDepth   int
	MaxWorkers int
	Strategy   DecomposeStrategy
	AutoSelect *bool // When true or nil, Strategy is chosen automatically via SelectStrategy.
}

func (c OperatorConfig) withDefaults() OperatorConfig {
	if c.MaxDepth <= 0 {
		c.MaxDepth = defaultMaxDepth
	}
	if c.MaxWorkers <= 0 {
		c.MaxWorkers = defaultMaxWorkers
	}
	if c.AutoSelect == nil {
		v := c.Strategy == 0
		c.AutoSelect = &v
	}
	return c
}

// Operator performs recursive task decomposition using a StructuredSubagent
// for each subtask. It supports four decomposition strategies and enforces
// depth limits and cycle detection.
type Operator struct {
	cfg        OperatorConfig
	executor   SubagentExecutor
	decomposer Decomposer
	visited    *sync.Map
}

// NewOperator creates an Operator with the given config, executor, and
// decomposer. The executor typically wraps StructuredSubagent.Execute.
func NewOperator(cfg OperatorConfig, executor SubagentExecutor, decomposer Decomposer) *Operator {
	return &Operator{
		cfg:        cfg.withDefaults(),
		executor:   executor,
		decomposer: decomposer,
		visited:    &sync.Map{},
	}
}

// Run executes the task using the configured strategy. It decomposes the
// task, runs subtasks, and aggregates results. Depth starts at 0 and
// increments with each recursive call.
func (op *Operator) Run(ctx context.Context, task string, context map[string]string) OperatorResult {
	return op.run(ctx, task, context, 0)
}

func (op *Operator) run(ctx context.Context, task string, context map[string]string, depth int) OperatorResult {
	if err := op.checkGuardrails(task, depth); err != nil {
		return OperatorResult{
			Strategy: op.cfg.Strategy,
			Depth:    depth,
			Error:    err.Error(),
		}
	}

	sig := taskSignature(task, context)
	if _, loaded := op.visited.LoadOrStore(sig, true); loaded {
		return OperatorResult{
			Strategy: op.cfg.Strategy,
			Depth:    depth,
			Error:    fmt.Sprintf("cycle detected: task %q already visited", truncate(task, 40)),
		}
	}

	if !op.isDecomposable(task) {
		resp, err := op.executor(ctx, StructuredRequest{
			Task:    task,
			Context: context,
		})
		if err != nil {
			return OperatorResult{Strategy: op.cfg.Strategy, Depth: depth, Error: err.Error()}
		}
		return OperatorResult{
			Success:  resp.Success,
			Result:   resp.Result,
			Depth:    depth,
			Strategy: op.cfg.Strategy,
		}
	}

	subtasks, err := op.decomposer.Decompose(ctx, task, context)
	if err != nil {
		return OperatorResult{
			Strategy: op.cfg.Strategy,
			Depth:    depth,
			Error:    fmt.Sprintf("decompose failed: %v", err),
		}
	}

	if len(subtasks) == 0 {
		resp, err := op.executor(ctx, StructuredRequest{Task: task, Context: context})
		if err != nil {
			return OperatorResult{Strategy: op.cfg.Strategy, Depth: depth, Error: err.Error()}
		}
		return OperatorResult{Success: resp.Success, Result: resp.Result, Depth: depth, Strategy: op.cfg.Strategy}
	}

	if op.cfg.AutoSelect != nil && *op.cfg.AutoSelect {
		op.cfg.Strategy = SelectStrategy(subtasks)
	}

	switch op.cfg.Strategy {
	case StrategyLLMMap:
		return op.runLLMMap(ctx, subtasks, depth)
	case StrategyAgenticMap:
		return op.runAgenticMap(ctx, subtasks, depth)
	case StrategyBatch:
		return op.runBatch(ctx, subtasks, depth)
	case StrategySequential:
		return op.runSequential(ctx, subtasks, depth)
	default:
		return OperatorResult{Strategy: op.cfg.Strategy, Depth: depth, Error: fmt.Sprintf("unknown strategy: %d", op.cfg.Strategy)}
	}
}

func (op *Operator) checkGuardrails(task string, depth int) error {
	if depth > op.cfg.MaxDepth {
		return fmt.Errorf("max recursion depth %d exceeded", op.cfg.MaxDepth)
	}
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("empty task")
	}
	return nil
}

func (op *Operator) isDecomposable(task string) bool {
	return len(strings.TrimSpace(task)) >= minTaskLength
}

func (op *Operator) runLLMMap(ctx context.Context, subtasks []Subtask, depth int) OperatorResult {
	return op.runParallel(ctx, subtasks, depth, func(_ Subtask) map[string]string {
		return nil
	})
}

func (op *Operator) runAgenticMap(ctx context.Context, subtasks []Subtask, depth int) OperatorResult {
	return op.runParallel(ctx, subtasks, depth, func(st Subtask) map[string]string {
		return st.Context
	})
}

func (op *Operator) runParallel(ctx context.Context, subtasks []Subtask, depth int, contextFn func(Subtask) map[string]string) OperatorResult {
	workers := min(len(subtasks), op.cfg.MaxWorkers)

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	results := make([]StructuredResponse, len(subtasks))
	errs := make([]string, len(subtasks))

	for i, st := range subtasks {
		wg.Add(1)
		go func(idx int, subtask Subtask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			req := StructuredRequest{
				Task:    subtask.Task,
				Context: contextFn(subtask),
				Tools:   subtask.Tools,
			}
			resp, err := op.executor(ctx, req)
			if err != nil {
				errs[idx] = err.Error()
				return
			}
			results[idx] = resp
		}(i, st)
	}
	wg.Wait()

	return op.aggregateResults(results, errs, depth)
}

func (op *Operator) runBatch(ctx context.Context, subtasks []Subtask, depth int) OperatorResult {
	sharedContext := make(map[string]string)
	maps.Copy(sharedContext, subtasks[0].Context)

	var subResults []StructuredResponse
	for _, st := range subtasks {
		for k, v := range sharedContext {
			if st.Context == nil {
				st.Context = make(map[string]string)
			}
			st.Context[k] = v
		}

		resp, err := op.executor(ctx, StructuredRequest{
			Task:    st.Task,
			Context: st.Context,
			Tools:   st.Tools,
		})
		if err != nil {
			return OperatorResult{
				Strategy:   op.cfg.Strategy,
				Depth:      depth,
				Error:      err.Error(),
				SubResults: subResults,
			}
		}
		subResults = append(subResults, resp)

		if resp.Success {
			sharedContext[fmt.Sprintf("subtask_%s_result", st.ID)] = resp.Result
		}
	}

	return op.aggregateFromResponses(subResults, depth)
}

func (op *Operator) runSequential(ctx context.Context, subtasks []Subtask, depth int) OperatorResult {
	var subResults []StructuredResponse
	prevOutput := ""

	for i, st := range subtasks {
		subCtx := make(map[string]string)
		maps.Copy(subCtx, st.Context)
		if prevOutput != "" {
			subCtx["previous_output"] = prevOutput
		}

		resp, err := op.executor(ctx, StructuredRequest{
			Task:    st.Task,
			Context: subCtx,
			Tools:   st.Tools,
		})
		if err != nil {
			return OperatorResult{
				Strategy:   op.cfg.Strategy,
				Depth:      depth,
				Error:      err.Error(),
				SubResults: subResults,
			}
		}
		subResults = append(subResults, resp)

		if !resp.Success {
			return OperatorResult{
				Success:    false,
				Result:     resp.Result,
				SubResults: subResults,
				Depth:      depth,
				Strategy:   op.cfg.Strategy,
				Error:      fmt.Sprintf("subtask %d failed: %s", i, resp.Error),
			}
		}
		prevOutput = resp.Result
	}

	return op.aggregateFromResponses(subResults, depth)
}

func (op *Operator) aggregateResults(results []StructuredResponse, errs []string, depth int) OperatorResult {
	var subResults []StructuredResponse
	var parts []string
	var errParts []string
	allSuccess := true

	for i, r := range results {
		if errs[i] != "" {
			allSuccess = false
			errParts = append(errParts, errs[i])
			parts = append(parts, fmt.Sprintf("[error: %s]", errs[i]))
			continue
		}
		subResults = append(subResults, r)
		if !r.Success {
			allSuccess = false
			errParts = append(errParts, r.Error)
		}
		if r.Result != "" {
			parts = append(parts, r.Result)
		}
	}

	errMsg := ""
	if len(errParts) > 0 {
		errMsg = strings.Join(errParts, "; ")
	}

	return OperatorResult{
		Success:    allSuccess,
		Result:     strings.Join(parts, "\n"),
		SubResults: subResults,
		Depth:      depth,
		Strategy:   op.cfg.Strategy,
		Error:      errMsg,
	}
}

func (op *Operator) aggregateFromResponses(subResults []StructuredResponse, depth int) OperatorResult {
	allSuccess := true
	var parts []string
	for _, r := range subResults {
		if !r.Success {
			allSuccess = false
		}
		if r.Result != "" {
			parts = append(parts, r.Result)
		}
	}

	return OperatorResult{
		Success:    allSuccess,
		Result:     strings.Join(parts, "\n"),
		SubResults: subResults,
		Depth:      depth,
		Strategy:   op.cfg.Strategy,
	}
}

func taskSignature(task string, context map[string]string) string {
	h := sha256.New()
	io.WriteString(h, task)
	io.WriteString(h, "\x00")
	for k, v := range context {
		io.WriteString(h, k)
		io.WriteString(h, "=")
		io.WriteString(h, v)
		io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SelectStrategy analyzes subtasks and returns the most appropriate
// DecomposeStrategy based on their characteristics.
func SelectStrategy(subtasks []Subtask) DecomposeStrategy {
	if len(subtasks) == 0 {
		return StrategyLLMMap
	}

	allReadOnly := true
	for _, st := range subtasks {
		if !isReadOnlyTools(st.Tools) {
			allReadOnly = false
			break
		}
	}
	if allReadOnly {
		return StrategyLLMMap
	}

	hasSequential := false
	for _, st := range subtasks {
		if st.Context != nil {
			for k := range st.Context {
				if strings.HasPrefix(k, "subtask_") || strings.HasPrefix(k, "previous_output") {
					hasSequential = true
					break
				}
			}
		}
		if hasSequential {
			break
		}
	}
	if hasSequential {
		return StrategySequential
	}

	if allSimilarTools(subtasks) {
		return StrategyBatch
	}

	return StrategyAgenticMap
}

// readOnlyTools is the set of tools that don't modify files.
var readOnlyTools = map[string]bool{
	"view": true, "grep": true, "glob": true, "ls": true,
	"diagnostics": true, "references": true, "definition": true,
}

func isReadOnlyTools(tools []string) bool {
	if len(tools) == 0 {
		return false
	}
	for _, t := range tools {
		if !readOnlyTools[t] {
			return false
		}
	}
	return true
}

func allSimilarTools(subtasks []Subtask) bool {
	if len(subtasks) < 2 {
		return false
	}
	first := toolSet(subtasks[0].Tools)
	for _, st := range subtasks[1:] {
		if !sameToolSet(first, toolSet(st.Tools)) {
			return false
		}
	}
	return true
}

func toolSet(tools []string) map[string]bool {
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		set[t] = true
	}
	return set
}

func sameToolSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
