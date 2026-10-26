package api

import (
	"fmt"
	"io"
	"time"
)

const (
	maxErrorBodyBytes    int64 = 1 << 20
	maxModelsBodyBytes   int64 = 16 << 20
	maxResponseBodyBytes int64 = 64 << 20

	// fallbackDrainTimeout bounds how long an error body is drained for
	// connection reuse before the caller moves on to the next URL.
	fallbackDrainTimeout = time.Second

	// terminalChunkGrace bounds delivery of a final stream message after the
	// request context has expired, so a timeout is reported even though the
	// context can no longer be used for sending.
	terminalChunkGrace = time.Second
)

func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return body, nil
}

// drainErrorBody reads up to limit bytes from body and reports whether the whole
// body was read. Draining is best-effort: an oversized, truncated, or stalled
// error body returns false instead of an error, so callers can fall back to the
// status line rather than aborting an endpoint fallback chain. The body stays
// owned by the caller, which must close it (that also unblocks a timed-out read).
func drainErrorBody(body io.Reader, limit int64) ([]byte, bool) {
	type drainResult struct {
		body []byte
		err  error
	}

	done := make(chan drainResult, 1)
	go func() {
		b, err := readAllLimited(body, limit)
		done <- drainResult{body: b, err: err}
	}()

	timer := time.NewTimer(fallbackDrainTimeout)
	defer timer.Stop()

	select {
	case result := <-done:
		if result.err != nil {
			return nil, false
		}
		return result.body, true
	case <-timer.C:
		return nil, false
	}
}
