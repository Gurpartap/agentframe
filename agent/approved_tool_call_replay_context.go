package agent

import "context"

// ApprovedToolCallReplayOverride binds a resumed tool approval to an exact tool call replay target.
type ApprovedToolCallReplayOverride struct {
	ToolCallID  string
	Fingerprint string
}

type approvedToolCallReplayOverrideContextKey struct{}

// WithApprovedToolCallReplayOverride attaches a replay override payload to context.
func WithApprovedToolCallReplayOverride(ctx context.Context, payload ApprovedToolCallReplayOverride) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, approvedToolCallReplayOverrideContextKey{}, payload)
}

// ApprovedToolCallReplayOverrideFromContext reads a replay override payload from context.
func ApprovedToolCallReplayOverrideFromContext(ctx context.Context) (ApprovedToolCallReplayOverride, bool) {
	if ctx == nil {
		return ApprovedToolCallReplayOverride{}, false
	}
	payload, ok := ctx.Value(approvedToolCallReplayOverrideContextKey{}).(ApprovedToolCallReplayOverride)
	return payload, ok
}
