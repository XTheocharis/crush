//go:build !((darwin && (amd64 || arm64)) || (freebsd && (amd64 || arm64)) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (windows && (386 || amd64 || arm64)))

package db

import (
	"regexp"

	"github.com/ncruces/go-sqlite3"
)

func init() {
	sqlite3.AutoExtension(func(c *sqlite3.Conn) error {
		return c.CreateFunction("regexp", 2, sqlite3.DETERMINISTIC, func(ctx sqlite3.Context, args ...sqlite3.Value) {
			if len(args) != 2 {
				ctx.ResultInt(0)
				return
			}
			pattern := args[0].Text()
			text := args[1].Text()
			matched, err := regexp.MatchString(pattern, text)
			if err != nil || !matched {
				ctx.ResultInt(0)
				return
			}
			ctx.ResultInt(1)
		})
	})
}
