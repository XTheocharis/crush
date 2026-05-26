package agent

import (
	"context"
)

const repomapRefreshHookName = "repomap-refresh-trigger"

// RepoMapRefresh triggers an asynchronous repo map refresh for the given
// session by invoking the repomap extension's OnRunStart hook. If the
// extension host or hook is not available, it returns nil (graceful
// degradation).
func (c *coordinator) RepoMapRefresh(ctx context.Context, sessionID string) error {
	if c.extHost == nil {
		return nil
	}
	for _, hook := range c.extHost.RunHooks() {
		if hook.Name == repomapRefreshHookName && hook.OnRunStart != nil {
			return hook.OnRunStart(ctx, sessionID, "")
		}
	}
	return nil
}
