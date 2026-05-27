package stdlib

var rustCrates = toSet([]string{
	"alloc", "core", "std", "proc_macro", "test",
})

func IsRustStdlib(crateName string) bool {
	return rustCrates[crateName]
}
