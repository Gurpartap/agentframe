package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Gurpartap/agentframe/examples/coding-agent/client/internal/api"
	"github.com/Gurpartap/agentframe/examples/coding-agent/client/internal/events"
)

const eventsReconnectDelay = 150 * time.Millisecond

func runEvents(ctx context.Context, baseURL string, jsonMode bool, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("events requires <run-id>")
	}

	runID := strings.TrimSpace(args[0])
	if runID == "" {
		return errors.New("events requires <run-id>")
	}

	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	cursor := fs.Int64("cursor", 0, "stream cursor")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return errors.New("events accepts one run-id and flags only")
	}
	if *cursor < 0 {
		return errors.New("cursor must be >= 0")
	}

	client := &http.Client{}
	currentCursor := *cursor

	for {
		streamBody, err := openEventsStream(ctx, client, baseURL, runID, currentCursor)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return err
		}

		reader := events.NewReader(streamBody)
		needReconnect := false
		for {
			frame, raw, err := reader.Next()
			if err != nil {
				_ = streamBody.Close()
				if errors.Is(err, io.EOF) {
					needReconnect = true
					break
				}
				return fmt.Errorf("read events stream: %w", err)
			}

			if frame.ID <= currentCursor {
				_ = streamBody.Close()
				return fmt.Errorf("read events stream: non-monotonic id=%d cursor=%d", frame.ID, currentCursor)
			}
			currentCursor = frame.ID

			if jsonMode {
				if err := writeRaw(stdout, raw); err != nil {
					_ = streamBody.Close()
					return err
				}
				continue
			}

			if err := writeHumanEvent(stdout, frame); err != nil {
				_ = streamBody.Close()
				return err
			}
		}

		if !needReconnect {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(eventsReconnectDelay):
		}
	}
}

func openEventsStream(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	runID string,
	cursor int64,
) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	escapedRunID := url.PathEscape(strings.TrimSpace(runID))
	streamURL := fmt.Sprintf("%s/v1/runs/%s/events?cursor=%d", trimmedBaseURL, escapedRunID, cursor)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new events request: %w", err)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("open events stream: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read events error body: %w", readErr)
		}
		return nil, decodeEventsOpenError(response.StatusCode, body)
	}

	return response.Body, nil
}

func decodeEventsOpenError(statusCode int, body []byte) error {
	var parsed api.ErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil && strings.TrimSpace(parsed.Error.Code) != "" {
		return fmt.Errorf(
			"events stream rejected: status=%d code=%s message=%s",
			statusCode,
			parsed.Error.Code,
			parsed.Error.Message,
		)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		trimmed = http.StatusText(statusCode)
	}
	return fmt.Errorf("events stream rejected: status=%d message=%s", statusCode, trimmed)
}

func writeHumanEvent(out io.Writer, frame events.StreamEvent) error {
	_, err := fmt.Fprintf(
		out,
		"id=%d run_id=%s step=%d type=%s description=%s\n",
		frame.ID,
		frame.Event.RunID,
		frame.Event.Step,
		frame.Event.Type,
		strings.TrimSpace(frame.Event.Description),
	)
	return err
}
