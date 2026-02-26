package toolset

import (
	"bytes"
	"context"
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
			return "", &agent.SuspendRequestError{
				Requirement: &agent.PendingRequirement{
					ID:         fmt.Sprintf("req-bash-policy-%s", call.ID),
					Kind:       agent.RequirementKindApproval,
					Origin:     agent.RequirementOriginTool,
					ToolCallID: call.ID,
					Prompt:     "approve bash command denied by policy",
				},
				Err: err,
			}
		}
		return "", err
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
