package lsp

import (
	"fmt"
	"os/exec"
)

// DetectNodeJS finds the node binary on PATH.
func DetectNodeJS() (string, error) {
	return exec.LookPath("node")
}

// DetectNpm finds the npm binary on PATH.
func DetectNpm() (string, error) {
	return exec.LookPath("npm")
}

// DetectPython finds python3 or python on PATH.
func DetectPython() (string, error) {
	if p, err := exec.LookPath("python3"); err == nil {
		return p, nil
	}
	return exec.LookPath("python")
}

// DetectUvx finds uvx or uv on PATH.
func DetectUvx() (string, error) {
	if p, err := exec.LookPath("uvx"); err == nil {
		return p, nil
	}
	return exec.LookPath("uv")
}

// DetectGo finds the go binary on PATH.
func DetectGo() (string, error) {
	return exec.LookPath("go")
}

// DetectRuntime dispatches to the appropriate detector based on dep name.
// Returns ("", nil) for empty dep (no dependency = always available).
func DetectRuntime(dep string) (string, error) {
	switch dep {
	case "":
		return "", nil
	case "node":
		return DetectNodeJS()
	case "npm":
		return DetectNpm()
	case "python":
		return DetectPython()
	case "uvx":
		return DetectUvx()
	case "go":
		return DetectGo()
	default:
		return "", fmt.Errorf("runtime detection: unknown runtime dependency %q", dep)
	}
}

// IsRuntimeAvailable returns true if the runtime is available, false otherwise.
// Returns true for empty dep (no dependency needed).
func IsRuntimeAvailable(dep string) bool {
	_, err := DetectRuntime(dep)
	return err == nil
}
