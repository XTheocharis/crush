//go:build (darwin && (amd64 || arm64)) || (freebsd && (amd64 || arm64)) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (windows && (386 || amd64 || arm64))

package db

import (
	"database/sql/driver"
	"regexp"

	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(
		"regexp",
		2,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			pattern, ok := args[0].(string)
			if !ok {
				return int64(0), nil
			}
			text, ok := args[1].(string)
			if !ok {
				return int64(0), nil
			}
			matched, err := regexp.MatchString(pattern, text)
			if err != nil {
				return int64(0), nil
			}
			if matched {
				return int64(1), nil
			}
			return int64(0), nil
		},
	)
}
