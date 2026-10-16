//go:build !solaris

package summon

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// panicError is a panic raised while polling, turned into an ordinary error.
// It deliberately does not wrap the panic value: a panic that carries, say, a
// Conjur 401 must not be classified as an authentication failure.
type panicError struct {
	value any
}

func (e *panicError) Error() string {
	return fmt.Sprintf("panic (%T): %s", e.value, panicText(e.value))
}

func panicText(v any) string {
	switch v := v.(type) {
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

// recoveredPanic converts a value returned by recover() into a panicError and
// logs its stack at DEBUG only. Call it from the deferred function that
// recovered, so the stack still shows the panicking frames.
func recoveredPanic(where string, v any) error {
	err := &panicError{value: v}
	slog.Debug("Recovered panic", "where", where, "panic", err.Error(), "stack", string(debug.Stack()))
	return err
}

// recoverFetch wraps fetch so that a panic inside it, whichever goroutine it
// runs on, becomes an error returned to the caller.
func recoverFetch(fetch secretFetcher) secretFetcher {
	if fetch == nil {
		return nil
	}
	return func(ctx context.Context, id string) (value []byte, err error) {
		defer func() {
			if v := recover(); v != nil {
				value, err = nil, recoveredPanic("secret fetch", v)
			}
		}()
		return fetch(ctx, id)
	}
}
