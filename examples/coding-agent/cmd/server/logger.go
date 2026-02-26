package main

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
	"github.com/lmittmann/tint"
)

var serverLogOutput io.Writer = os.Stderr

func newServerLogger(output io.Writer, level slog.Leveler, format config.LogFormat) *slog.Logger {
	if level == nil {
		level = slog.LevelInfo
	}
	if output == nil {
		output = os.Stderr
	}

	switch config.LogFormat(strings.ToLower(strings.TrimSpace(string(format)))) {
	case config.LogFormatJSON:
		return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
			Level:     level,
			AddSource: false,
		}))
	default:
		handler := tint.NewHandler(output, &tint.Options{
			Level:      level,
			AddSource:  false,
			TimeFormat: "2006-01-02 15:04:05.000Z07:00",
			NoColor:    false,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Value.Kind() == slog.KindAny {
					if _, ok := a.Value.Any().(error); ok {
						return tint.Attr(9, a)
					}
				}
				return a
			},
		})
		return slog.New(handler)
	}
}
