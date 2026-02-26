package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

var serverLogOutput io.Writer = os.Stderr

func newServerLogger(output io.Writer) *slog.Logger {
	handler := tint.NewHandler(output, &tint.Options{
		Level:      slog.LevelInfo,
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
