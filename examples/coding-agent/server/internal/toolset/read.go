package toolset

import (
	"fmt"
	"os"
)

func (e *Executor) executeRead(arguments map[string]any) (string, error) {
	path, err := stringArgument(arguments, "path")
	if err != nil {
		return "", err
	}

	resolved, err := e.policy.ResolvePath(path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read %q: path is a directory", path)
	}
	if info.Size() > e.policy.maxReadSize {
		return "", fmt.Errorf("read %q: file size %d exceeds limit %d", path, info.Size(), e.policy.maxReadSize)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return string(content), nil
}
