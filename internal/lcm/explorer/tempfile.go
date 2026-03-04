package explorer

import "os"

// withTempFile creates a temporary file with the given prefix and content,
// closes the file, calls fn with its path, and removes the file on return.
// The file is always cleaned up, even when fn returns an error.
func withTempFile(prefix string, content []byte, fn func(path string) error) error {
	f, err := os.CreateTemp("", prefix)
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)

	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return fn(path)
}
