package stdlib

var nodeModules = toSet([]string{
	"assert", "buffer", "child_process", "crypto",
	"events", "fs", "http", "https", "net",
	"os", "path", "stream", "timers", "url",
	"util", "zlib",
})

func IsNodeStdlib(module string) bool {
	return nodeModules[module]
}
