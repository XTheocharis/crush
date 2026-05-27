package stdlib

var cppHeaders = toSet([]string{
	"algorithm", "array", "chrono", "filesystem",
	"functional", "future", "iostream", "map",
	"memory", "optional", "set", "span",
	"string", "string_view", "tuple", "unordered_map",
	"unordered_set", "utility", "variant", "vector",
})

func IsCppStdlib(header string) bool {
	return cppHeaders[header]
}
