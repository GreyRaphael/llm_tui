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

	// fallbackDrainTimeout bounds how long a 404/405 error body is drained for
	// connection reuse before the fallback chain moves on to the next URL.
	fallbackDrainTimeout = time.Second
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
