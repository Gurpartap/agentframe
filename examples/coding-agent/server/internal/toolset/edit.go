package toolset

import (
	"fmt"
	"os"
	"strings"
)

func (e *Executor) executeEdit(arguments map[string]any) (string, error) {
	path, err := stringArgument(arguments, "path")
	if err != nil {
		return "", err
	}
	oldValue, err := stringArgument(arguments, "old")
	if err != nil {
		return "", err
	}
	newValue, err := stringArgument(arguments, "new")
	if err != nil {
		return "", err
	}

	resolved, err := e.policy.ResolvePath(path)
	if err != nil {
		return "", err
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("edit %q: read: %w", path, err)
	}

	content := string(raw)
	if !strings.Contains(content, oldValue) {
		return "", fmt.Errorf("edit %q: target text not found", path)
	}

	updated := strings.Replace(content, oldValue, newValue, 1)
	if err := os.WriteFile(resolved, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("edit %q: write: %w", path, err)
	}

	return fmt.Sprintf("edit_ok path=%s replacements=1", path), nil
}
