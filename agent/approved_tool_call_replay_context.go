package agent

import "context"

// ApprovedToolCallReplayOverride binds a resumed tool approval to an exact tool call replay target.
type ApprovedToolCallReplayOverride struct {
	ToolCallID  string
	Fingerprint string
}

type approvedToolCallReplayOverrideContextKey struct{}

type approvedToolCallReplayOverrideContextValue struct {
	override ApprovedToolCallReplayOverride
	present  bool
}

// WithApprovedToolCallReplayOverride attaches a replay override payload to context.
func WithApprovedToolCallReplayOverride(ctx context.Context, payload ApprovedToolCallReplayOverride) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, approvedToolCallReplayOverrideContextKey{}, approvedToolCallReplayOverrideContextValue{
		override: payload,
		present:  true,
	})
}

// WithoutApprovedToolCallReplayOverride clears replay override payload from context.
func WithoutApprovedToolCallReplayOverride(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, approvedToolCallReplayOverrideContextKey{}, approvedToolCallReplayOverrideContextValue{})
}

// ApprovedToolCallReplayOverrideFromContext reads a replay override payload from context.
func ApprovedToolCallReplayOverrideFromContext(ctx context.Context) (ApprovedToolCallReplayOverride, bool) {
	if ctx == nil {
		return ApprovedToolCallReplayOverride{}, false
	}
	payload, ok := ctx.Value(approvedToolCallReplayOverrideContextKey{}).(approvedToolCallReplayOverrideContextValue)
	if !ok || !payload.present {
		return ApprovedToolCallReplayOverride{}, false
	}
	return payload.override, true
}
