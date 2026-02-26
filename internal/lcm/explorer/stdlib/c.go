package stdlib

var cHeaders = toSet([]string{
	"assert", "ctype", "errno", "float", "limits",
	"locale", "math", "setjmp", "signal", "stdarg",
	"stdbool", "stddef", "stdint", "stdio",
	"stdlib", "string", "time",
})

func IsCStdlib(header string) bool {
	return cHeaders[header]
}
