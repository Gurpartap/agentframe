package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	allowedRequirementKinds = []string{"approval", "user_input", "external_execution"}
	allowedOutcomes         = []string{"approved", "rejected", "provided", "completed"}
)

type ResolutionInput struct {
	RequirementID string
	Kind          string
	Outcome       string
	Value         string
}

type ResolutionPromptDefaults struct {
	RequirementID string
	Kind          string
	Prompt        string
}

type ResolutionRequiredError struct {
	defaults ResolutionPromptDefaults
}

func NewResolutionRequiredError(defaults ResolutionPromptDefaults) *ResolutionRequiredError {
	return &ResolutionRequiredError{defaults: defaults}
}

func (e *ResolutionRequiredError) Defaults() ResolutionPromptDefaults {
	if e == nil {
		return ResolutionPromptDefaults{}
	}
	return e.defaults
}

func (e *ResolutionRequiredError) Error() string {
	if e == nil {
		return "run requires resolution"
	}
	if strings.TrimSpace(e.defaults.Prompt) != "" {
		return "run requires resolution: " + strings.TrimSpace(e.defaults.Prompt)
	}
	return "run requires resolution"
}

func PromptResolution(
	ctx context.Context,
	in *bufio.Reader,
	renderer *Renderer,
	defaults ResolutionPromptDefaults,
) (*ResolutionInput, error) {
	if in == nil {
		return nil, errors.New("resolution prompt: input reader is required")
	}
	if renderer == nil {
		return nil, errors.New("resolution prompt: renderer is required")
	}

	if err := renderer.PrintLine("run is suspended and requires a resolution payload"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(defaults.Prompt) != "" {
		if err := renderer.PrintLine("requirement prompt: " + strings.TrimSpace(defaults.Prompt)); err != nil {
			return nil, err
		}
	}

	requirementID, err := promptField(ctx, in, renderer, "requirement_id", defaults.RequirementID, true, nil)
	if err != nil {
		return nil, err
	}

	kind, err := promptField(
		ctx,
		in,
		renderer,
		"kind (approval|user_input|external_execution)",
		defaults.Kind,
		true,
		ValidateRequirementKind,
	)
	if err != nil {
		return nil, err
	}

	outcome, err := promptField(
		ctx,
		in,
		renderer,
		"outcome (approved|rejected|provided|completed)",
		"",
		true,
		ValidateResolutionOutcome,
	)
	if err != nil {
		return nil, err
	}

	value, err := promptField(ctx, in, renderer, "value (optional)", "", false, nil)
	if err != nil {
		return nil, err
	}

	return &ResolutionInput{
		RequirementID: requirementID,
		Kind:          kind,
		Outcome:       outcome,
		Value:         value,
	}, nil
}

func ValidateRequirementKind(kind string) error {
	normalized := strings.TrimSpace(kind)
	for _, allowed := range allowedRequirementKinds {
		if normalized == allowed {
			return nil
		}
	}
	return fmt.Errorf("unsupported requirement kind %q (allowed: %s)", normalized, strings.Join(allowedRequirementKinds, ", "))
}

func ValidateResolutionOutcome(outcome string) error {
	normalized := strings.TrimSpace(outcome)
	for _, allowed := range allowedOutcomes {
		if normalized == allowed {
			return nil
		}
	}
	return fmt.Errorf("unsupported resolution outcome %q (allowed: %s)", normalized, strings.Join(allowedOutcomes, ", "))
}

func promptField(
	ctx context.Context,
	in *bufio.Reader,
	renderer *Renderer,
	label string,
	defaultValue string,
	required bool,
	validate func(string) error,
) (string, error) {
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		promptLabel := label
		if strings.TrimSpace(defaultValue) != "" {
			promptLabel += " [" + strings.TrimSpace(defaultValue) + "]"
		}
		if err := renderer.PrintLine(promptLabel + ":"); err != nil {
			return "", err
		}
		if err := renderer.ShowPrompt(); err != nil {
			return "", err
		}
		line, err := in.ReadString('\n')
		renderer.HidePrompt()
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}

		value := strings.TrimSpace(line)
		if value == "" {
			value = strings.TrimSpace(defaultValue)
		}

		if required && value == "" {
			if writeErr := renderer.PrintLine("field is required"); writeErr != nil {
				return "", writeErr
			}
			if errors.Is(err, io.EOF) {
				return "", errors.New("resolution prompt ended before required field was provided")
			}
			continue
		}

		if value != "" && validate != nil {
			if validateErr := validate(value); validateErr != nil {
				if writeErr := renderer.PrintLine(validateErr.Error()); writeErr != nil {
					return "", writeErr
				}
				if errors.Is(err, io.EOF) {
					return "", validateErr
				}
				continue
			}
		}

		return value, nil
	}
}
