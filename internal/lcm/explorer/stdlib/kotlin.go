package stdlib

import "strings"

var kotlinPackages = toSet([]string{
	"kotlin", "kotlin.annotation", "kotlin.collections",
	"kotlin.comparisons", "kotlin.concurrent", "kotlin.coroutines",
	"kotlin.coroutines.cancellation", "kotlin.coroutines.channels",
	"kotlin.coroutines.flow", "kotlin.coroutines.intrinsics",
	"kotlin.coroutines.jvm", "kotlin.coroutines.selects",
	"kotlin.experimental", "kotlin.io", "kotlin.jvm",
	"kotlin.math", "kotlin.native", "kotlin.properties",
	"kotlin.ranges", "kotlin.random", "kotlin.reflect",
	"kotlin.sequences", "kotlin.statemachine", "kotlin.system",
	"kotlin.text", "kotlin.time", "kotlinx.coroutines",
})

func IsKotlinStdlib(packageName string) bool {
	prefix := strings.Split(packageName, ".")[0]
	return kotlinPackages[prefix] || kotlinPackages[packageName]
}
