package clientevents

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReaderParsesNDJSONFrames(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(
		"\n" +
			`{"id":1,"event":{"run_id":"run-1","step":0,"type":"run_started"}}` + "\n" +
			`{"id":2,"event":{"run_id":"run-1","step":1,"type":"assistant_message"}}` + "\n",
	)

	reader := NewReader(input)

	first, rawFirst, err := reader.Next()
	if err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if first.ID != 1 {
		t.Fatalf("first id mismatch: got=%d want=%d", first.ID, 1)
	}
	if first.Event.Type != "run_started" {
		t.Fatalf("first type mismatch: got=%q want=%q", first.Event.Type, "run_started")
	}
	if string(rawFirst) != `{"id":1,"event":{"run_id":"run-1","step":0,"type":"run_started"}}`+"\n" {
		t.Fatalf("first raw mismatch: %q", string(rawFirst))
	}

	second, rawSecond, err := reader.Next()
	if err != nil {
		t.Fatalf("read second frame: %v", err)
	}
	if second.ID != 2 {
		t.Fatalf("second id mismatch: got=%d want=%d", second.ID, 2)
	}
	if second.Event.Type != "assistant_message" {
		t.Fatalf("second type mismatch: got=%q want=%q", second.Event.Type, "assistant_message")
	}
	if string(rawSecond) != `{"id":2,"event":{"run_id":"run-1","step":1,"type":"assistant_message"}}`+"\n" {
		t.Fatalf("second raw mismatch: %q", string(rawSecond))
	}

	_, _, err = reader.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReaderParsesSSEDataFrames(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(
		"data: " + `{"id":3,"event":{"run_id":"run-2","step":2,"type":"run_checkpoint"}}` + "\n\n",
	)

	reader := NewReader(input)
	frame, raw, err := reader.Next()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if frame.ID != 3 {
		t.Fatalf("id mismatch: got=%d want=%d", frame.ID, 3)
	}
	if frame.Event.RunID != "run-2" {
		t.Fatalf("run_id mismatch: got=%q want=%q", frame.Event.RunID, "run-2")
	}
	if string(raw) != `{"id":3,"event":{"run_id":"run-2","step":2,"type":"run_checkpoint"}}`+"\n" {
		t.Fatalf("raw mismatch: %q", string(raw))
	}
}

func TestReaderInvalidPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid json",
			input: "{not-json}\n",
		},
		{
			name:  "missing run id",
			input: `{"id":1,"event":{"step":0,"type":"run_started"}}` + "\n",
		},
		{
			name:  "invalid id",
			input: `{"id":0,"event":{"run_id":"run-1","step":0,"type":"run_started"}}` + "\n",
		},
		{
			name:  "invalid sse field",
			input: "event: message\n\n",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reader := NewReader(strings.NewReader(tc.input))
			_, _, err := reader.Next()
			if err == nil {
				t.Fatalf("expected parse error")
			}
		})
	}
}
