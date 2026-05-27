package agent

import (
	"github.com/charmbracelet/crush/internal/config"
)

const (
	// DefaultSmallModelTokenLimit is the default token count threshold.
	// Requests with token counts at or below this limit are routed to the
	// editor model; requests above are routed to the architect model.
	DefaultSmallModelTokenLimit = 4000

	// charsPerToken is the estimated number of characters per token used
	// for character-based token estimation.
	charsPerToken = 4
)

// ModelRouter determines which model role should handle a request based on
// the estimated token count of the input. This is a standalone utility —
// coordinator integration is not required at this layer.
type ModelRouter struct {
	// SmallModelTokenLimit is the threshold in tokens. Inputs at or below
	// this count are routed to the editor model; inputs above go to the
	// architect model. When zero, DefaultSmallModelTokenLimit is used.
	SmallModelTokenLimit int
}

// NewModelRouter creates a ModelRouter with default settings.
func NewModelRouter() *ModelRouter {
	return &ModelRouter{
		SmallModelTokenLimit: DefaultSmallModelTokenLimit,
	}
}

// NewModelRouterWithLimit creates a ModelRouter with a custom token limit.
func NewModelRouterWithLimit(limit int) *ModelRouter {
	return &ModelRouter{
		SmallModelTokenLimit: limit,
	}
}

// limit returns the effective token limit, falling back to the default when
// the configured value is zero.
func (r *ModelRouter) limit() int {
	if r.SmallModelTokenLimit <= 0 {
		return DefaultSmallModelTokenLimit
	}
	return r.SmallModelTokenLimit
}

// RouteByTokenCount returns the appropriate model role for the given token
// count. Token counts at or below the limit return RoleEditor; counts above
// return RoleArchitect.
func (r *ModelRouter) RouteByTokenCount(tokenCount int) config.ModelRole {
	if tokenCount <= r.limit() {
		return config.RoleEditor
	}
	return config.RoleArchitect
}

// RouteByCharCount converts a character count to an estimated token count
// using ceiling division, then delegates to RouteByTokenCount.
func (r *ModelRouter) RouteByCharCount(charCount int) config.ModelRole {
	tokenCount := (charCount + charsPerToken - 1) / charsPerToken
	return r.RouteByTokenCount(tokenCount)
}
