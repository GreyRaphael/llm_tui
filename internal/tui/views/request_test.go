package views

import "testing"

func TestRequestIDsAreUniqueAcrossScreenInstances(t *testing.T) {
	first := nextRequestID()
	second := nextRequestID()
	if first == 0 || second == 0 || first == second {
		t.Fatalf("expected distinct non-zero request IDs, got %d and %d", first, second)
	}
}
