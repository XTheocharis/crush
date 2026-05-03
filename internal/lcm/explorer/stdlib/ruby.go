package stdlib

var rubyModules = toSet([]string{
	"csv", "fileutils", "json", "net/http",
	"ostruct", "pathname", "securerandom", "set",
	"time", "uri", "yaml",
})

func IsRubyStdlib(module string) bool {
	return rubyModules[module]
}
