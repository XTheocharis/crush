package agent

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"charm.land/fantasy"
)

// RateLimitCoordinator coordinates backoff across concurrent LLM calls that
// share the same provider. When one call receives a 429, all other concurrent
// calls to the same provider will respect the shared backoff rather than
// independently racing and compounding the rate limit.
//
// Normal (non-rate-limited) calls are NOT serialized — they proceed concurrently.
// Coordination only kicks in when a 429 is observed.
type RateLimitCoordinator struct {
	mu       sync.Mutex
	backoffs map[string]*providerBackoff
}

type providerBackoff struct {
	until time.Time
}

// NewRateLimitCoordinator creates a new RateLimitCoordinator.
func NewRateLimitCoordinator() *RateLimitCoordinator {
	return &RateLimitCoordinator{}
}

// WaitIfBackedOff blocks until the provider's backoff period expires or the
// context is cancelled. Returns immediately if no backoff is active.
func (c *RateLimitCoordinator) WaitIfBackedOff(ctx context.Context, provider string) error {
	c.mu.Lock()
	bo, exists := c.backoffs[provider]
	if !exists || time.Now().After(bo.until) {
		c.mu.Unlock()
		return nil
	}
	waitDuration := time.Until(bo.until)
	c.mu.Unlock()

	if waitDuration <= 0 {
		return nil
	}

	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RecordRateLimit extracts the retry-after duration from a 429 ProviderError
// and sets the shared backoff for the given provider. Other concurrent callers
// will see this backoff and wait before making their next request.
func (c *RateLimitCoordinator) RecordRateLimit(provider string, err *fantasy.ProviderError) {
	delay := extractRetryAfter(err)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.backoffs == nil {
		c.backoffs = make(map[string]*providerBackoff)
	}

	bo, exists := c.backoffs[provider]
	if !exists {
		bo = &providerBackoff{}
		c.backoffs[provider] = bo
	}

	newUntil := time.Now().Add(delay)
	if newUntil.After(bo.until) {
		bo.until = newUntil
	}
}

type rateLimitedModel struct {
	inner    fantasy.LanguageModel
	coord    *RateLimitCoordinator
	provider string
}

func newRateLimitedModel(
	inner fantasy.LanguageModel,
	coord *RateLimitCoordinator,
	provider string,
) fantasy.LanguageModel {
	return &rateLimitedModel{
		inner:    inner,
		coord:    coord,
		provider: provider,
	}
}

func (m *rateLimitedModel) Generate(
	ctx context.Context, call fantasy.Call,
) (*fantasy.Response, error) {
	if err := m.coord.WaitIfBackedOff(ctx, m.provider); err != nil {
		return nil, err
	}
	resp, err := m.inner.Generate(ctx, call)
	m.recordIfRateLimit(err)
	return resp, err
}

func (m *rateLimitedModel) Stream(
	ctx context.Context, call fantasy.Call,
) (fantasy.StreamResponse, error) {
	if err := m.coord.WaitIfBackedOff(ctx, m.provider); err != nil {
		return nil, err
	}
	resp, err := m.inner.Stream(ctx, call)
	m.recordIfRateLimit(err)
	return resp, err
}

func (m *rateLimitedModel) GenerateObject(
	ctx context.Context, call fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	if err := m.coord.WaitIfBackedOff(ctx, m.provider); err != nil {
		return nil, err
	}
	resp, err := m.inner.GenerateObject(ctx, call)
	m.recordIfRateLimit(err)
	return resp, err
}

func (m *rateLimitedModel) StreamObject(
	ctx context.Context, call fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	if err := m.coord.WaitIfBackedOff(ctx, m.provider); err != nil {
		return nil, err
	}
	resp, err := m.inner.StreamObject(ctx, call)
	m.recordIfRateLimit(err)
	return resp, err
}

func (m *rateLimitedModel) Provider() string { return m.inner.Provider() }
func (m *rateLimitedModel) Model() string    { return m.inner.Model() }

func (m *rateLimitedModel) recordIfRateLimit(err error) {
	if err == nil {
		return
	}
	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) &&
		providerErr.StatusCode == http.StatusTooManyRequests {
		m.coord.RecordRateLimit(m.provider, providerErr)
	}
}

// extractRetryAfter extracts the retry-after duration from a ProviderError's
// response headers. Falls back to a reasonable default if no header is found.
func extractRetryAfter(err *fantasy.ProviderError) time.Duration {
	if err.ResponseHeaders != nil {
		if ms := err.ResponseHeaders["retry-after-ms"]; ms != "" {
			if millis, e := strconv.ParseInt(ms, 10, 64); e == nil && millis > 0 {
				return time.Duration(millis) * time.Millisecond
			}
		}
		if ra := err.ResponseHeaders["retry-after"]; ra != "" {
			if secs, e := strconv.ParseInt(ra, 10, 64); e == nil && secs > 0 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	// Matches fantasy's default initial backoff.
	return 2 * time.Second
}
