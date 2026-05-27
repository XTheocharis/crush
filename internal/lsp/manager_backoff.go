package lsp

// [XRUSH: begin: exponential backoff unavailable tracking]
func (s *Manager) recentlyUnavailable(name string) bool { // XRUSH: rewritten for exponential backoff
	state, exists := s.unavailable.Get(name)
	if !exists {
		return false
	}
	delay := s.backoff.NextInterval(state.AttemptCount)
	return s.now().Sub(state.LastAttempt) < delay
}

func (s *Manager) markUnavailable(name string) { // XRUSH: rewritten for exponential backoff
	state, exists := s.unavailable.Get(name)
	if exists {
		state.AttemptCount++
		state.LastAttempt = s.now()
	} else {
		s.unavailable.Set(name, &serverRetryState{
			LastAttempt:  s.now(),
			AttemptCount: 0,
		})
	}
}

// [XRUSH: end]
