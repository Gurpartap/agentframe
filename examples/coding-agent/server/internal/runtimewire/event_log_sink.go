package runtimewire

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
)

type runtimeEventLogSink struct {
	logger    *slog.Logger
	logFormat config.LogFormat
}

func newRuntimeEventLogSink(logger *slog.Logger, logFormat config.LogFormat) agent.EventSink {
	if logger == nil {
		return nil
	}
	if logFormat == "" {
		logFormat = config.LogFormatText
	}
	return runtimeEventLogSink{
		logger:    logger,
		logFormat: logFormat,
	}
}

func (s runtimeEventLogSink) Publish(ctx context.Context, event agent.Event) error {
	if ctx == nil {
		return agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	if s.logFormat == config.LogFormatJSON {
		s.logger.Debug("run event", slog.Any("event", event))
		return nil
	}

	eventPayload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		return marshalErr
	}

	s.logger.Debug("run event", slog.String("event", string(eventPayload)))
	return nil
}
