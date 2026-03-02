package agent

import (
	"sync/atomic"

	"charm.land/fantasy"
)

// validateLCMSubAgentParams checks that scope-reduction params are provided
// when LCM is active and the calling session is itself a sub-agent.
// Returns a non-nil ToolResponse error when validation fails.
func (c *coordinator) validateLCMSubAgentParams(sessionID string, params AgentParams) *fantasy.ToolResponse {
	if c.lcm == nil || !c.sessions.IsAgentToolSession(sessionID) {
		return nil
	}
	if params.DelegatedScope == "" || params.KeptWork == "" {
		resp := fantasy.NewTextErrorResponse("delegated_scope and kept_work are required when spawning sub-agents with LCM active")
		return &resp
	}
	return nil
}

func (a *sessionAgent) SetPrepareStepHooks(hooks []PrepareStepHook) {
	a.prepareStepHooks.SetSlice(hooks)
}

func (a *sessionAgent) currentQueueGeneration(sessionID string) int64 {
	counter, ok := a.queueGenerationBySID.Get(sessionID)
	if !ok || counter == nil {
		counter = &atomic.Int64{}
		a.queueGenerationBySID.Set(sessionID, counter)
	}
	return counter.Load()
}

func (a *sessionAgent) incrementQueueGeneration(sessionID string) int64 {
	counter, ok := a.queueGenerationBySID.Get(sessionID)
	if !ok || counter == nil {
		counter = &atomic.Int64{}
		a.queueGenerationBySID.Set(sessionID, counter)
	}
	return counter.Add(1)
}
