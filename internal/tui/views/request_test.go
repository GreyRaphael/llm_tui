package views

import (
	"context"
	"testing"
)

func TestRequestIDsAreUniqueAcrossScreenInstances(t *testing.T) {
	first := nextRequestID()
	second := nextRequestID()
	if first == 0 || second == 0 || first == second {
		t.Fatalf("expected distinct non-zero request IDs, got %d and %d", first, second)
	}
}

func TestTrackedRequestLifecycle(t *testing.T) {
	var cancel context.CancelFunc
	var id uint64

	ctx, first := beginTrackedRequest(&cancel, &id)
	if first == 0 || id != first || ctx.Err() != nil {
		t.Fatalf("unexpected initial request state: id=%d ctxErr=%v", id, ctx.Err())
	}
	if finishTrackedRequest(&cancel, &id, first+1) {
		t.Fatal("stale message id must not complete the active request")
	}
	if ctx.Err() != nil {
		t.Fatal("stale message id must not cancel the active request")
	}
	if !finishTrackedRequest(&cancel, &id, first) {
		t.Fatal("matching message id must complete the active request")
	}
	if ctx.Err() == nil {
		t.Fatal("completing a request must release its context")
	}
	if id != 0 || cancel != nil {
		t.Fatalf("completion must clear request state, got id=%d cancel=%v", id, cancel != nil)
	}
	if finishTrackedRequest(&cancel, &id, 0) {
		t.Fatal("zero message id must never be treated as current")
	}
}

func TestBeginTrackedRequestSupersedesPrevious(t *testing.T) {
	var cancel context.CancelFunc
	var id uint64

	firstCtx, firstID := beginTrackedRequest(&cancel, &id)
	secondCtx, secondID := beginTrackedRequest(&cancel, &id)
	if firstCtx.Err() == nil {
		t.Fatal("starting a new request must cancel the previous one")
	}
	if secondID == firstID || id != secondID || secondCtx.Err() != nil {
		t.Fatalf("unexpected superseded request state: first=%d second=%d id=%d", firstID, secondID, id)
	}
	cancelTrackedRequest(&cancel, &id)
	if secondCtx.Err() == nil || id != 0 || cancel != nil {
		t.Fatal("cancelTrackedRequest did not release the active request")
	}
	// Cancelling twice must be safe.
	cancelTrackedRequest(&cancel, &id)
}
