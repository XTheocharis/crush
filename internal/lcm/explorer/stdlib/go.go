package stdlib

func IsGoStdlib(path string) bool {
	for _, r := range path {
		if r == '.' {
			return false
		}
	}
	return path != ""
}
