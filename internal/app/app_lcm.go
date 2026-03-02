package app

import (
	"database/sql"
	"log/slog"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/treesitter"
)

// initLCM initializes the LCM manager and wraps the message service with
// the LCM decorator when LCM is configured. Returns the (possibly wrapped)
// message service and the LCM manager (nil when LCM is not configured).
func initLCM(cfg *config.Config, q *db.Queries, conn *sql.DB, messages message.Service) (message.Service, lcm.Manager) {
	if cfg.Options.LCM == nil {
		return messages, nil
	}
	mgr := lcm.NewManager(q, conn)
	messages = lcm.NewMessageDecorator(messages, mgr, q, conn, lcm.MessageDecoratorConfig{
		DisableLargeToolOutput:        cfg.Options.LCM.DisableLargeToolOutput,
		LargeToolOutputTokenThreshold: cfg.Options.LCM.LargeToolOutputTokenThreshold,
		Parser:                        treesitter.NewParser(),
		ExplorerOutputProfile:         lcmExplorerOutputProfile(cfg.Options.LCM.ExplorerOutputProfile),
	})
	slog.Info("LCM enabled")
	return messages, mgr
}
