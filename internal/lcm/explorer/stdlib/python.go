package stdlib

var pythonModules = toSet([]string{
	"abc", "argparse", "asyncio", "collections",
	"contextlib", "dataclasses", "datetime", "enum",
	"functools", "itertools", "json", "logging",
	"math", "os", "pathlib", "re", "subprocess",
	"sys", "threading", "time", "typing",
})

func IsPythonStdlib(module string) bool {
	return pythonModules[module]
}
