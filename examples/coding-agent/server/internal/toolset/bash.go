package toolset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Gurpartap/agentframe/agent"
)

func (e *Executor) executeBash(ctx context.Context, call agent.ToolCall) (string, error) {
	command, err := stringArgument(call.Arguments, "command")
	if err != nil {
		return "", err
	}
	if err := e.policy.ValidateBashCommand(command); err != nil {
		if errors.Is(err, ErrBashCommandDenied) {
			fingerprint := bashApprovalFingerprint(call, command, e.policy)
			if err := validateApprovedBashReplay(ctx, call.ID, fingerprint); err != nil {
				return "", err
			}
			if !approvedBashReplay(ctx, call.ID, fingerprint) {
				return "", &agent.SuspendRequestError{
					Requirement: &agent.PendingRequirement{
						ID:          fmt.Sprintf("req-bash-policy-%s", call.ID),
						Kind:        agent.RequirementKindApproval,
						Origin:      agent.RequirementOriginTool,
						ToolCallID:  call.ID,
						Fingerprint: fingerprint,
						Prompt:      "approve bash command denied by policy",
					},
					Err: err,
				}
			}
		} else {
			return "", err
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, e.policy.BashTimeout())
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-lc", command)
	cmd.Dir = e.policy.WorkspaceRoot()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf(
			"%w: command=%q timeout=%s stdout=%q stderr=%q",
			ErrBashExecutionTimedOut,
			command,
			e.policy.BashTimeout(),
			stdout.String(),
			stderr.String(),
		)
	}
	if err != nil {
		return "", fmt.Errorf(
			"bash command %q failed: %w stdout=%q stderr=%q",
			command,
			err,
			stdout.String(),
			stderr.String(),
		)
	}

	return fmt.Sprintf(
		"bash_ok command=%q stdout=%q stderr=%q",
		command,
		strings.TrimSpace(stdout.String()),
		strings.TrimSpace(stderr.String()),
	), nil
}

func bashApprovalFingerprint(call agent.ToolCall, command string, policy Policy) string {
	payload, _ := json.Marshal(struct {
		ToolName      string `json:"tool_name"`
		CallID        string `json:"call_id"`
		Command       string `json:"command"`
		WorkspaceRoot string `json:"workspace_root"`
		BashTimeoutNS int64  `json:"bash_timeout_ns"`
	}{
		ToolName:      call.Name,
		CallID:        call.ID,
		Command:       strings.TrimSpace(command),
		WorkspaceRoot: policy.WorkspaceRoot(),
		BashTimeoutNS: int64(policy.BashTimeout()),
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func approvedBashReplay(ctx context.Context, callID, fingerprint string) bool {
	override, ok := agent.ApprovedToolCallReplayOverrideFromContext(ctx)
	if !ok {
		return false
	}
	return override.ToolCallID == callID && override.Fingerprint == fingerprint
}

func validateApprovedBashReplay(ctx context.Context, callID, fingerprint string) error {
	override, ok := agent.ApprovedToolCallReplayOverrideFromContext(ctx)
	if !ok {
		return nil
	}
	if override.ToolCallID == callID && override.Fingerprint == fingerprint {
		return nil
	}
	return fmt.Errorf(
		"%w: field=approved_tool_replay_override reason=mismatch got_tool_call_id=%q got_fingerprint=%q want_tool_call_id=%q want_fingerprint=%q",
		ErrBashReplayMismatch,
		override.ToolCallID,
		override.Fingerprint,
		callID,
		fingerprint,
	)
}
