package views

import (
	"context"
	"sync/atomic"
)

var requestSequence atomic.Uint64

func nextRequestID() uint64 {
	return requestSequence.Add(1)
}

func beginTrackedRequest(cancel *context.CancelFunc, id *uint64) (context.Context, uint64) {
	cancelTrackedRequest(cancel, id)
	ctx, cancelFn := context.WithCancel(context.Background())
	*cancel = cancelFn
	*id = nextRequestID()
	return ctx, *id
}

func finishTrackedRequest(cancel *context.CancelFunc, currentID *uint64, messageID uint64) bool {
	if messageID == 0 || *currentID != messageID {
		return false
	}
	cancelTrackedRequest(cancel, currentID)
	return true
}

func cancelTrackedRequest(cancel *context.CancelFunc, id *uint64) {
	if *cancel != nil {
		(*cancel)()
		*cancel = nil
	}
	*id = 0
}
