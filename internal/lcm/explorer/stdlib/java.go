package stdlib

import "strings"

// javaPackagePrefixes contains the top-level package prefixes that belong
// to the Java standard library and JDK modules.
var javaPackagePrefixes = []string{
	"java.",
	"javax.",
	"jdk.",
}

// IsJavaStdlib reports whether an import path belongs to the Java
// standard library or JDK modules.
func IsJavaStdlib(importPath string) bool {
	importPath = strings.TrimSpace(importPath)
	for _, prefix := range javaPackagePrefixes {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}
