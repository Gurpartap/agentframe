package toolset_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/toolset"
)

func TestPolicyResolvePathRejectsEscape(t *testing.T) {
	t.Parallel()

	policy, err := toolset.NewPolicy(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	if _, err := policy.ResolvePath("../outside.txt"); !errors.Is(err, toolset.ErrPathOutsideWorkspace) {
		t.Fatalf("expected ErrPathOutsideWorkspace, got %v", err)
	}
}

func TestPolicyResolvePathRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlink permission constraints on windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	policy, err := toolset.NewPolicy(root, time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	if _, err := policy.ResolvePath("escape/file.txt"); !errors.Is(err, toolset.ErrPathOutsideWorkspace) {
		t.Fatalf("expected ErrPathOutsideWorkspace, got %v", err)
	}
}

func TestExecutorReadWriteEditRoundTrip(t *testing.T) {
	t.Parallel()

	policy, err := toolset.NewPolicy(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	executor := toolset.NewExecutor(policy)

	ctx := context.Background()
	if _, err := executor.Execute(ctx, agent.ToolCall{
		ID:   "write-1",
		Name: toolset.ToolWrite,
		Arguments: map[string]any{
			"path":    "notes.txt",
			"content": "hello toolset\n",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	readResult, err := executor.Execute(ctx, agent.ToolCall{
		ID:   "read-1",
		Name: toolset.ToolRead,
		Arguments: map[string]any{
			"path": "notes.txt",
		},
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(readResult.Content, "hello toolset") {
		t.Fatalf("unexpected read content: %q", readResult.Content)
	}

	if _, err := executor.Execute(ctx, agent.ToolCall{
		ID:   "edit-1",
		Name: toolset.ToolEdit,
		Arguments: map[string]any{
			"path": "notes.txt",
			"old":  "hello toolset",
			"new":  "hello real tools",
		},
	}); err != nil {
		t.Fatalf("edit: %v", err)
	}

	bashResult, err := executor.Execute(ctx, agent.ToolCall{
		ID:   "bash-1",
		Name: toolset.ToolBash,
		Arguments: map[string]any{
			"command": "cat notes.txt",
		},
	})
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	if !strings.Contains(bashResult.Content, "hello real tools") {
		t.Fatalf("unexpected bash output: %q", bashResult.Content)
	}
}

func TestExecutorBashPolicyRejectsForbiddenToken(t *testing.T) {
	t.Parallel()

	policy, err := toolset.NewPolicy(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	executor := toolset.NewExecutor(policy)

	call := agent.ToolCall{
		ID:   "bash-denied-1",
		Name: toolset.ToolBash,
		Arguments: map[string]any{
			"command": "ls; pwd",
		},
	}
	_, err = executor.Execute(context.Background(), call)
	var suspendErr *agent.SuspendRequestError
	if !errors.As(err, &suspendErr) {
		t.Fatalf("expected SuspendRequestError, got %T (%v)", err, err)
	}
	if suspendErr.Requirement == nil {
		t.Fatalf("expected suspend requirement payload")
	}
	if suspendErr.Requirement.ID != "req-bash-policy-bash-denied-1" {
		t.Fatalf("requirement id mismatch: got=%q want=%q", suspendErr.Requirement.ID, "req-bash-policy-bash-denied-1")
	}
	if suspendErr.Requirement.Kind != agent.RequirementKindApproval {
		t.Fatalf("requirement kind mismatch: got=%s want=%s", suspendErr.Requirement.Kind, agent.RequirementKindApproval)
	}
	if suspendErr.Requirement.Origin != agent.RequirementOriginTool {
		t.Fatalf("requirement origin mismatch: got=%s want=%s", suspendErr.Requirement.Origin, agent.RequirementOriginTool)
	}
	if suspendErr.Requirement.ToolCallID != "bash-denied-1" {
		t.Fatalf("requirement tool_call_id mismatch: got=%q want=%q", suspendErr.Requirement.ToolCallID, "bash-denied-1")
	}
	if suspendErr.Requirement.Fingerprint == "" {
		t.Fatalf("requirement fingerprint must be populated for tool-origin suspension")
	}
	if !errors.Is(err, toolset.ErrBashCommandDenied) {
		t.Fatalf("expected ErrBashCommandDenied, got %v", err)
	}

	_, err = executor.Execute(context.Background(), call)
	var repeatedSuspendErr *agent.SuspendRequestError
	if !errors.As(err, &repeatedSuspendErr) {
		t.Fatalf("expected repeated SuspendRequestError, got %T (%v)", err, err)
	}
	if repeatedSuspendErr.Requirement == nil {
		t.Fatalf("expected repeated suspend requirement payload")
	}
	if repeatedSuspendErr.Requirement.Fingerprint != suspendErr.Requirement.Fingerprint {
		t.Fatalf(
			"fingerprint must be deterministic for same denied call: got=%q want=%q",
			repeatedSuspendErr.Requirement.Fingerprint,
			suspendErr.Requirement.Fingerprint,
		)
	}
}

func TestExecutorBashPolicyApprovedReplayOverrideBypassesSuspendRequest(t *testing.T) {
	t.Parallel()

	policy, err := toolset.NewPolicy(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	executor := toolset.NewExecutor(policy)

	call := agent.ToolCall{
		ID:   "bash-denied-replay-1",
		Name: toolset.ToolBash,
		Arguments: map[string]any{
			"command": "ls; pwd",
		},
	}

	_, err = executor.Execute(context.Background(), call)
	var suspendErr *agent.SuspendRequestError
	if !errors.As(err, &suspendErr) {
		t.Fatalf("expected SuspendRequestError, got %T (%v)", err, err)
	}
	if suspendErr.Requirement == nil {
		t.Fatalf("expected suspend requirement payload")
	}
	if suspendErr.Requirement.Fingerprint == "" {
		t.Fatalf("expected non-empty fingerprint")
	}

	override := agent.ApprovedToolCallReplayOverride{
		ToolCallID:  suspendErr.Requirement.ToolCallID,
		Fingerprint: suspendErr.Requirement.Fingerprint,
	}
	result, err := executor.Execute(agent.WithApprovedToolCallReplayOverride(context.Background(), override), call)
	if err != nil {
		t.Fatalf("replayed execution returned error: %v", err)
	}
	if !strings.Contains(result.Content, "bash_ok") {
		t.Fatalf("unexpected replayed content: %q", result.Content)
	}

	_, err = executor.Execute(context.Background(), call)
	var resuspendedErr *agent.SuspendRequestError
	if !errors.As(err, &resuspendedErr) {
		t.Fatalf("expected suspension after replay when override is absent, got %T (%v)", err, err)
	}
}

func TestExecutorBashPolicyReplayOverrideMismatchReturnsContractError(t *testing.T) {
	t.Parallel()

	policy, err := toolset.NewPolicy(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	executor := toolset.NewExecutor(policy)

	call := agent.ToolCall{
		ID:   "bash-denied-replay-mismatch-1",
		Name: toolset.ToolBash,
		Arguments: map[string]any{
			"command": "ls; pwd",
		},
	}

	_, err = executor.Execute(context.Background(), call)
	var suspendErr *agent.SuspendRequestError
	if !errors.As(err, &suspendErr) {
		t.Fatalf("expected SuspendRequestError, got %T (%v)", err, err)
	}
	if suspendErr.Requirement == nil {
		t.Fatalf("expected suspend requirement payload")
	}

	override := agent.ApprovedToolCallReplayOverride{
		ToolCallID:  suspendErr.Requirement.ToolCallID,
		Fingerprint: suspendErr.Requirement.Fingerprint + "-mismatch",
	}
	_, err = executor.Execute(agent.WithApprovedToolCallReplayOverride(context.Background(), override), call)
	if !errors.Is(err, toolset.ErrBashReplayMismatch) {
		t.Fatalf("expected ErrBashReplayMismatch, got %v", err)
	}
	if errors.Is(err, toolset.ErrBashCommandDenied) {
		t.Fatalf("mismatch override must not fall back to policy deny suspension: %v", err)
	}
	var mismatchSuspendErr *agent.SuspendRequestError
	if errors.As(err, &mismatchSuspendErr) {
		t.Fatalf("mismatch override must return contract error, got suspension payload")
	}
	if !strings.Contains(err.Error(), "approved_tool_replay_override") {
		t.Fatalf("unexpected mismatch error: %q", err.Error())
	}
}

func TestExecutorBashTimeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("tail -f /dev/null timeout scenario is unix-specific")
	}

	policy, err := toolset.NewPolicy(t.TempDir(), 150*time.Millisecond)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	executor := toolset.NewExecutor(policy)

	_, err = executor.Execute(context.Background(), agent.ToolCall{
		ID:   "bash-timeout-1",
		Name: toolset.ToolBash,
		Arguments: map[string]any{
			"command": "tail -f /dev/null",
		},
	})
	if !errors.Is(err, toolset.ErrBashExecutionTimedOut) {
		t.Fatalf("expected ErrBashExecutionTimedOut, got %v", err)
	}
}
