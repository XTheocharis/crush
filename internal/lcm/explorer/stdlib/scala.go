package stdlib

import "strings"

var scalaPackages = toSet([]string{
	"scala", "scala.annotation", "scala.annotation.unspecialized",
	"scala.beans", "scala.collection", "scala.collection.concurrent",
	"scala.collection.convert", "scala.collection.generic",
	"scala.collection.immutable", "scala.collection.mutable",
	"scala.collection.parallel", "scala.collection.parallel.immutable",
	"scala.collection.parallel.mutable", "scala.concurrent",
	"scala.io", "scala.math", "scala.reflect",
	"scala.runtime", "scala.sys", "scala.text",
	"scala.util", "scala.util.control",
	"scala.util.hashing", "scala.util.matching",
})

func IsScalaStdlib(packageName string) bool {
	prefix := strings.Split(packageName, ".")[0]
	return scalaPackages[prefix] || scalaPackages[packageName]
}
