package runtimewire

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Gurpartap/agentframe/agent"
)

type runtimeEventLogSink struct {
	logger *slog.Logger
}

func newRuntimeEventLogSink(logger *slog.Logger) agent.EventSink {
	if logger == nil {
		return nil
	}
	return runtimeEventLogSink{logger: logger}
}

func (s runtimeEventLogSink) Publish(ctx context.Context, event agent.Event) error {
	if ctx == nil {
		return agent.ErrContextNil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	eventPayload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	s.logger.Debug("run event", slog.String("event", string(eventPayload)))
	return nil
}
