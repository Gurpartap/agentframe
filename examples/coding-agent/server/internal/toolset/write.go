package toolset

import (
	"fmt"
	"os"
	"path/filepath"
)

func (e *Executor) executeWrite(arguments map[string]any) (string, error) {
	path, err := stringArgument(arguments, "path")
	if err != nil {
		return "", err
	}
	content, err := stringArgument(arguments, "content")
	if err != nil {
		return "", err
	}

	resolved, err := e.policy.ResolvePath(path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("write %q: create parent directory: %w", path, err)
	}

	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %q: %w", path, err)
	}

	return fmt.Sprintf("write_ok path=%s bytes=%d", path, len(content)), nil
}
