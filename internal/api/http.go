package api

import (
	"fmt"
	"io"
)

const (
	maxErrorBodyBytes    int64 = 1 << 20
	maxModelsBodyBytes   int64 = 16 << 20
	maxResponseBodyBytes int64 = 64 << 20
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
