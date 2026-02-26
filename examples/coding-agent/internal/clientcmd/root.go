package clientcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientconfig"
)

const usageText = `Usage:
  coding-agent-client [global flags] <command> [args]

Commands:
  chat
  health
  start --user-prompt <text> [--run-id <id>] [--system-prompt <text>] [--max-steps <n>]
  get <run-id>
  events <run-id> [--cursor <n>]
  continue <run-id> [--max-steps <n>] [--requirement-id <id> --kind <kind> --outcome <outcome> [--value <value>]]
  steer <run-id> --instruction <text>
  follow-up <run-id> --prompt <text> [--max-steps <n>]
  cancel <run-id>

Global flags:
  --base-url <url>
  --token <token>
  --json
  --timeout <duration>
`

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return executeWithInput(ctx, args, os.Stdin, stdout, stderr)
}

func executeWithInput(ctx context.Context, args []string, input io.Reader, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := clientconfig.Load()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("coding-agent-client", flag.ContinueOnError)
	fs.SetOutput(stderr)

	baseURL := fs.String("base-url", cfg.BaseURL, "server base URL")
	token := fs.String("token", cfg.AuthToken, "bearer token for mutating routes")
	jsonMode := fs.Bool("json", cfg.JSON, "print raw response payload")
	timeout := fs.Duration("timeout", cfg.Timeout, "HTTP timeout (for example 10s)")

	fs.Usage = func() {
		_, _ = io.WriteString(stderr, usageText)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg = clientconfig.Config{
		BaseURL:   strings.TrimSpace(*baseURL),
		AuthToken: strings.TrimSpace(*token),
		JSON:      *jsonMode,
		Timeout:   *timeout,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fs.Usage()
		return errors.New("command is required")
	}

	api, err := clientapi.New(cfg.BaseURL, cfg.AuthToken, &http.Client{Timeout: cfg.Timeout})
	if err != nil {
		return err
	}

	command := remaining[0]
	commandArgs := remaining[1:]

	switch command {
	case "chat":
		return runChat(ctx, api, cfg.BaseURL, input, stdout)
	case "health":
		return runHealth(ctx, api, cfg.JSON, commandArgs, stdout)
	case "start":
		return runStart(ctx, api, cfg.JSON, commandArgs, stdout)
	case "get":
		return runGet(ctx, api, cfg.JSON, commandArgs, stdout)
	case "events":
		return runEvents(ctx, cfg.BaseURL, cfg.JSON, commandArgs, stdout)
	case "continue":
		return runContinue(ctx, api, cfg.JSON, commandArgs, stdout)
	case "steer":
		return runSteer(ctx, api, cfg.JSON, commandArgs, stdout)
	case "follow-up":
		return runFollowUp(ctx, api, cfg.JSON, commandArgs, stdout)
	case "cancel":
		return runCancel(ctx, api, cfg.JSON, commandArgs, stdout)
	default:
		fs.Usage()
		return fmt.Errorf("unsupported command %q", command)
	}
}

func runHealth(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("health does not accept arguments")
	}

	raw, err := api.Health(ctx)
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	_, err = fmt.Fprintln(stdout, trimmed)
	return err
}

func runStart(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	runID := fs.String("run-id", "", "run identifier")
	systemPrompt := fs.String("system-prompt", "", "system prompt")
	userPrompt := fs.String("user-prompt", "", "user prompt")
	maxSteps := fs.Int("max-steps", -1, "max command steps")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return errors.New("start does not accept positional arguments")
	}
	if strings.TrimSpace(*userPrompt) == "" {
		return errors.New("start requires --user-prompt")
	}

	request := clientapi.StartRequest{
		RunID:        strings.TrimSpace(*runID),
		SystemPrompt: *systemPrompt,
		UserPrompt:   *userPrompt,
	}
	optionalMaxSteps, err := parseOptionalMaxSteps(*maxSteps)
	if err != nil {
		return err
	}
	request.MaxSteps = optionalMaxSteps

	state, raw, err := api.Start(ctx, request)
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func runGet(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("get requires <run-id>")
	}

	state, raw, err := api.Get(ctx, strings.TrimSpace(args[0]))
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func runContinue(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("continue requires <run-id>")
	}
	runID := strings.TrimSpace(args[0])

	fs := flag.NewFlagSet("continue", flag.ContinueOnError)
	maxSteps := fs.Int("max-steps", -1, "max command steps")
	requirementID := fs.String("requirement-id", "", "requirement identifier")
	kind := fs.String("kind", "", "requirement kind")
	outcome := fs.String("outcome", "", "resolution outcome")
	value := fs.String("value", "", "resolution value")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return errors.New("continue accepts one run-id and flags only")
	}

	request := clientapi.ContinueRequest{}
	optionalMaxSteps, err := parseOptionalMaxSteps(*maxSteps)
	if err != nil {
		return err
	}
	request.MaxSteps = optionalMaxSteps

	resolutionFlagsSet := strings.TrimSpace(*requirementID) != "" || strings.TrimSpace(*kind) != "" || strings.TrimSpace(*outcome) != "" || strings.TrimSpace(*value) != ""
	if resolutionFlagsSet {
		if strings.TrimSpace(*requirementID) == "" || strings.TrimSpace(*kind) == "" || strings.TrimSpace(*outcome) == "" {
			return errors.New("continue resolution requires --requirement-id, --kind, and --outcome")
		}
		request.Resolution = &clientapi.Resolution{
			RequirementID: strings.TrimSpace(*requirementID),
			Kind:          strings.TrimSpace(*kind),
			Outcome:       strings.TrimSpace(*outcome),
			Value:         *value,
		}
	}

	state, raw, err := api.Continue(ctx, runID, request)
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func runSteer(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("steer requires <run-id>")
	}
	runID := strings.TrimSpace(args[0])

	fs := flag.NewFlagSet("steer", flag.ContinueOnError)
	instruction := fs.String("instruction", "", "steering instruction")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return errors.New("steer accepts one run-id and flags only")
	}
	if strings.TrimSpace(*instruction) == "" {
		return errors.New("steer requires --instruction")
	}

	state, raw, err := api.Steer(ctx, runID, clientapi.SteerRequest{
		Instruction: *instruction,
	})
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func runFollowUp(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("follow-up requires <run-id>")
	}
	runID := strings.TrimSpace(args[0])

	fs := flag.NewFlagSet("follow-up", flag.ContinueOnError)
	prompt := fs.String("prompt", "", "follow-up prompt")
	maxSteps := fs.Int("max-steps", -1, "max command steps")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return errors.New("follow-up accepts one run-id and flags only")
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("follow-up requires --prompt")
	}

	request := clientapi.FollowUpRequest{
		Prompt: *prompt,
	}
	optionalMaxSteps, err := parseOptionalMaxSteps(*maxSteps)
	if err != nil {
		return err
	}
	request.MaxSteps = optionalMaxSteps

	state, raw, err := api.FollowUp(ctx, runID, request)
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func runCancel(ctx context.Context, api *clientapi.Client, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("cancel requires <run-id>")
	}

	state, raw, err := api.Cancel(ctx, strings.TrimSpace(args[0]))
	if err != nil {
		return err
	}
	if jsonMode {
		return writeRaw(stdout, raw)
	}
	return writeRunState(stdout, state)
}

func parseOptionalMaxSteps(raw int) (*int, error) {
	if raw < -1 {
		return nil, errors.New("max-steps must be >= -1")
	}
	if raw == -1 {
		return nil, nil
	}
	value := raw
	return &value, nil
}

func writeRaw(out io.Writer, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	_, err := out.Write(body)
	return err
}

func writeRunState(out io.Writer, state clientapi.RunState) error {
	if _, err := fmt.Fprintf(out, "run_id: %s\n", state.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "status: %s\n", state.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "step: %d\n", state.Step); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "version: %d\n", state.Version); err != nil {
		return err
	}
	if state.Output != "" {
		if _, err := fmt.Fprintf(out, "output: %s\n", state.Output); err != nil {
			return err
		}
	}
	if state.Error != "" {
		if _, err := fmt.Fprintf(out, "error: %s\n", state.Error); err != nil {
			return err
		}
	}
	if state.PendingRequirement != nil {
		if _, err := fmt.Fprintf(out, "pending_requirement.id: %s\n", state.PendingRequirement.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "pending_requirement.kind: %s\n", state.PendingRequirement.Kind); err != nil {
			return err
		}
		if state.PendingRequirement.Prompt != "" {
			if _, err := fmt.Fprintf(out, "pending_requirement.prompt: %s\n", state.PendingRequirement.Prompt); err != nil {
				return err
			}
		}
	}
	return nil
}
