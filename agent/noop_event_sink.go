package agent

import "context"

type noopEventSink struct{}

func (noopEventSink) Publish(context.Context, Event) error {
	return nil
}
