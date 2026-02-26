package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Gurpartap/agentframe/agent"
)

const maxFrameBytes = 1024 * 1024

type StreamEvent struct {
	ID    int64       `json:"id"`
	Event agent.Event `json:"event"`
}

type Reader struct {
	scanner *bufio.Scanner
}

func NewReader(source io.Reader) *Reader {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 4096), maxFrameBytes)
	return &Reader{scanner: scanner}
}

func (r *Reader) Next() (StreamEvent, []byte, error) {
	if r == nil || r.scanner == nil {
		return StreamEvent{}, nil, io.EOF
	}

	for {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return StreamEvent{}, nil, err
			}
			return StreamEvent{}, nil, io.EOF
		}

		line := strings.TrimSpace(r.scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		payload, err := r.parsePayload(line)
		if err != nil {
			return StreamEvent{}, nil, err
		}

		var frame StreamEvent
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return StreamEvent{}, nil, fmt.Errorf("decode stream event: %w", err)
		}
		if frame.ID <= 0 {
			return StreamEvent{}, nil, fmt.Errorf("decode stream event: invalid id %d", frame.ID)
		}
		if strings.TrimSpace(string(frame.Event.RunID)) == "" {
			return StreamEvent{}, nil, fmt.Errorf("decode stream event: run_id is required")
		}

		raw := append([]byte(payload), '\n')
		return frame, raw, nil
	}
}

func (r *Reader) parsePayload(firstLine string) (string, error) {
	if !strings.HasPrefix(firstLine, "data:") {
		return firstLine, nil
	}

	data := strings.TrimSpace(strings.TrimPrefix(firstLine, "data:"))
	for r.scanner.Scan() {
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			break
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			return "", fmt.Errorf("decode stream event: unsupported SSE field %q", line)
		}

		part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			data = part
			continue
		}
		data += "\n" + part
	}

	if err := r.scanner.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(data) == "" {
		return "", fmt.Errorf("decode stream event: empty SSE data payload")
	}
	return data, nil
}
