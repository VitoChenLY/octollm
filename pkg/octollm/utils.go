package octollm

import "context"

func GetCtxValue[T any](ctx context.Context, key any) (T, bool) {
	var zero T
	if ctx == nil {
		return zero, false
	}
	raw := ctx.Value(key)
	if raw == nil {
		return zero, false
	}
	v, ok := raw.(T)
	if !ok {
		return zero, false
	}
	return v, true
}
