package toolset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Gurpartap/agentframe/agent"
)

const (
	ToolRead  = "read"
	ToolWrite = "write"
	ToolEdit  = "edit"
	ToolBash  = "bash"

	DefaultBashTimeout = 3 * time.Second
	DefaultMaxReadSize = 1 << 20
)

var (
	ErrPathRequired          = errors.New("tool path is required")
	ErrPathOutsideWorkspace  = errors.New("tool path escapes workspace root")
	ErrArgumentInvalid       = errors.New("tool arguments are invalid")
	ErrBashCommandEmpty      = errors.New("bash command is empty")
	ErrBashCommandDenied     = errors.New("bash command violates policy")
	ErrBashExecutionTimedOut = errors.New("bash command timed out")
)

var toolDefinitions = []agent.ToolDefinition{
	{
		Name:        ToolRead,
		Description: "Read a UTF-8 text file within the workspace root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []any{"path"},
		},
	},
	{
		Name:        ToolWrite,
		Description: "Write UTF-8 text content to a file within the workspace root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required": []any{"path", "content"},
		},
	},
	{
		Name:        ToolEdit,
		Description: "Replace the first occurrence of old text with new text in a file within the workspace root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
				"old":  map[string]any{"type": "string"},
				"new":  map[string]any{"type": "string"},
			},
			"required": []any{"path", "old", "new"},
		},
	},
	{
		Name:        ToolBash,
		Description: "Run a bounded command in the workspace root under command policy restrictions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []any{"command"},
		},
	},
}

var allowedBashCommands = map[string]struct{}{
	"cat":    {},
	"echo":   {},
	"find":   {},
	"grep":   {},
	"head":   {},
	"ls":     {},
	"pwd":    {},
	"rg":     {},
	"sed":    {},
	"stat":   {},
	"tail":   {},
	"wc":     {},
	"which":  {},
	"printf": {},
}

type Policy struct {
	workspaceRoot string
	bashTimeout   time.Duration
	maxReadSize   int64
}

func NewPolicy(workspaceRoot string, bashTimeout time.Duration) (Policy, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return Policy{}, fmt.Errorf("new tool policy: workspace root is required")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Policy{}, fmt.Errorf("new tool policy: resolve workspace root: %w", err)
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Policy{}, fmt.Errorf("new tool policy: workspace root does not exist: %q", rootAbs)
		}
		return Policy{}, fmt.Errorf("new tool policy: resolve workspace root symlinks: %w", err)
	}

	info, err := os.Stat(rootResolved)
	if err != nil {
		return Policy{}, fmt.Errorf("new tool policy: stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return Policy{}, fmt.Errorf("new tool policy: workspace root is not a directory: %q", rootResolved)
	}

	if bashTimeout <= 0 {
		bashTimeout = DefaultBashTimeout
	}

	return Policy{
		workspaceRoot: rootResolved,
		bashTimeout:   bashTimeout,
		maxReadSize:   DefaultMaxReadSize,
	}, nil
}

func (p Policy) WorkspaceRoot() string {
	return p.workspaceRoot
}

func (p Policy) BashTimeout() time.Duration {
	return p.bashTimeout
}

func (p Policy) ResolvePath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", ErrPathRequired
	}

	var candidate string
	if filepath.IsAbs(path) {
		candidate = filepath.Clean(path)
	} else {
		candidate = filepath.Join(p.workspaceRoot, filepath.Clean(path))
	}

	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}

	resolved, err := resolveExistingPrefix(candidateAbs)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}

	if !hasPathPrefix(p.workspaceRoot, resolved) {
		return "", fmt.Errorf("%w: %q", ErrPathOutsideWorkspace, path)
	}

	return candidateAbs, nil
}

func (p Policy) ValidateBashCommand(command string) error {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ErrBashCommandEmpty
	}

	forbiddenTokens := []string{"\n", "\r", ";", "&&", "||", "|", ">", "<", "`", "$", "(", ")"}
	for _, token := range forbiddenTokens {
		if strings.Contains(trimmed, token) {
			return fmt.Errorf("%w: forbidden token %q", ErrBashCommandDenied, token)
		}
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return ErrBashCommandEmpty
	}
	verb := parts[0]
	if _, allowed := allowedBashCommands[verb]; !allowed {
		return fmt.Errorf("%w: command %q is not allowed", ErrBashCommandDenied, verb)
	}
	return nil
}

func Definitions() []agent.ToolDefinition {
	return agent.CloneToolDefinitions(toolDefinitions)
}

type Executor struct {
	policy Policy
}

func NewExecutor(policy Policy) *Executor {
	return &Executor{policy: policy}
}

func (e *Executor) Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	if ctx == nil {
		return agent.ToolResult{}, agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.ToolResult{}, ctxErr
	}

	var (
		content string
		err     error
	)

	switch call.Name {
	case ToolRead:
		content, err = e.executeRead(call.Arguments)
	case ToolWrite:
		content, err = e.executeWrite(call.Arguments)
	case ToolEdit:
		content, err = e.executeEdit(call.Arguments)
	case ToolBash:
		content, err = e.executeBash(ctx, call)
	default:
		return agent.ToolResult{}, fmt.Errorf("toolset: unsupported tool %q", call.Name)
	}
	if err != nil {
		return agent.ToolResult{}, err
	}

	return agent.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
	}, nil
}

func stringArgument(arguments map[string]any, key string) (string, error) {
	if arguments == nil {
		return "", fmt.Errorf("%w: missing argument %q", ErrArgumentInvalid, key)
	}
	value, ok := arguments[key]
	if !ok {
		return "", fmt.Errorf("%w: missing argument %q", ErrArgumentInvalid, key)
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: argument %q must be a string", ErrArgumentInvalid, key)
	}
	if strings.TrimSpace(stringValue) == "" {
		return "", fmt.Errorf("%w: argument %q must not be empty", ErrArgumentInvalid, key)
	}
	return stringValue, nil
}

func resolveExistingPrefix(path string) (string, error) {
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			rel, relErr := filepath.Rel(current, path)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Clean(filepath.Join(resolved, rel)), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return filepath.Clean(path), nil
}

func hasPathPrefix(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..") && rel != ""
}
