package config

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  slog.Level
		ok    bool
	}{
		{name: "debug", input: "debug", want: slog.LevelDebug, ok: true},
		{name: "info", input: "info", want: slog.LevelInfo, ok: true},
		{name: "warn", input: "warn", want: slog.LevelWarn, ok: true},
		{name: "warning", input: "warning", want: slog.LevelWarn, ok: true},
		{name: "error", input: "error", want: slog.LevelError, ok: true},
		{name: "uppercase", input: "DEBUG", want: slog.LevelDebug, ok: true},
		{name: "invalid", input: "trace", ok: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			level, err := parseLogLevel(tc.input)
			if tc.ok {
				if err != nil {
					t.Fatalf("parseLogLevel(%q) error: %v", tc.input, err)
				}
				if level != tc.want {
					t.Fatalf("parseLogLevel(%q) mismatch: got=%s want=%s", tc.input, level, tc.want)
				}
				return
			}

			if err == nil {
				t.Fatalf("parseLogLevel(%q) expected error", tc.input)
			}
		})
	}
}

func TestParseLogFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  LogFormat
		ok    bool
	}{
		{name: "text", input: "text", want: LogFormatText, ok: true},
		{name: "json", input: "json", want: LogFormatJSON, ok: true},
		{name: "uppercase", input: "JSON", want: LogFormatJSON, ok: true},
		{name: "invalid", input: "pretty", ok: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			format, err := parseLogFormat(tc.input)
			if tc.ok {
				if err != nil {
					t.Fatalf("parseLogFormat(%q) error: %v", tc.input, err)
				}
				if format != tc.want {
					t.Fatalf("parseLogFormat(%q) mismatch: got=%q want=%q", tc.input, format, tc.want)
				}
				return
			}

			if err == nil {
				t.Fatalf("parseLogFormat(%q) expected error", tc.input)
			}
		})
	}
}
