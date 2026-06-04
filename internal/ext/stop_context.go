package ext

import "context"

type stopConditionKeyType struct{}

var stopConditionKey stopConditionKeyType

// WithStopCondition returns a context that carries the stop-condition flag.
func WithStopCondition(ctx context.Context, stopped bool) context.Context {
	return context.WithValue(ctx, stopConditionKey, stopped)
}

// StoppedByCondition reports whether the context indicates a stop-condition halt.
func StoppedByCondition(ctx context.Context) bool {
	v, _ := ctx.Value(stopConditionKey).(bool)
	return v
}
