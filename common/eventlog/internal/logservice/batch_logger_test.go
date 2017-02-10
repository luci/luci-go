// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logservice

import (
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	logpb "github.com/luci/luci-go/common/eventlog/proto"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

type testLogger struct {
	requests chan []*logpb.LogRequestLite_LogEventLite

	mu  sync.Mutex
	err error
}

func newTestLogger(err error) *testLogger {
	return &testLogger{
		requests: make(chan []*logpb.LogRequestLite_LogEventLite, 1),
		err:      err,
	}
}
func (tl *testLogger) LogSync(_ context.Context, events ...*logpb.LogRequestLite_LogEventLite) error {
	tl.requests <- events

	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.err
}

func expectLogCalled(t *testing.T, tl *testLogger, want []*logpb.LogRequestLite_LogEventLite) {
	select {
	case got := <-tl.requests:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("events: got: %v; want: %v", got, want)
		}
	case <-time.After(50 * time.Millisecond):
		t.Errorf("timed out waiting for upload")
	}
}

func expectNoMoreLogs(t *testing.T, tl *testLogger) {
	close(tl.requests) // Any future sends on this channel will panic.
	if len(tl.requests) != 0 {
		t.Errorf("unexpected send on tl.records: %v", <-tl.requests)
	}
}

func TestBatchLogger(t *testing.T) {
	ctx := context.Background()
	tl := newTestLogger(nil)
	tickc := make(chan time.Time)
	bl := newBatchLogger(ctx, tl, tickc)

	// We haven't logged any events yet.
	if len(tl.requests) != 0 {
		t.Errorf("events: got: %v; want: nil", tl.requests)
	}

	event := &logpb.LogRequestLite_LogEventLite{}
	bl.Log(event)
	bl.Log(event)

	// We have logged events, but upload hasn't been called
	if len(tl.requests) != 0 {
		t.Errorf("events: got: %v; want: nil", <-tl.requests)
	}

	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{event, event})

	// log another event.
	bl.Log(event)
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{event})
	expectNoMoreLogs(t, tl)
}

var errBang = errors.New("bang")

func TestRetries(t *testing.T) {
	// Each batch of events has 4 chances at being uploaded: one initial attempt and up to 3 retries.

	ctx := context.Background()
	tl := newTestLogger(retryError{errBang})
	tickc := make(chan time.Time)
	bl := newBatchLogger(ctx, tl, tickc)

	e1 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(1)}
	bl.Log(e1)

	// e1 attempt #1 fails.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1})

	// e1 attempt #2 fails.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1})

	// Now add e2.
	e2 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(2)}
	bl.Log(e2)

	// e1 attempt #3 fails; e2 attempt #1 fails.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1, e2})

	// final attempt for e1
	// e1 attempt #4 (final) fails; e2 attempt #2 fails.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1, e2})

	tl.mu.Lock()
	tl.err = nil // start succeeding.  We've already given up on e1 though.
	tl.mu.Unlock()

	// e2 attempt #3 succeeds.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e2})

	// e2 already succeeded, so there should be no attempt #4.
	expectNoMoreLogs(t, tl)
	tickc <- time.Time{} // trigger upload.
}

func TestAttemptsUploadOnClose(t *testing.T) {
	ctx := context.Background()
	tl := newTestLogger(retryError{errBang})
	tickc := make(chan time.Time)
	bl := newBatchLogger(ctx, tl, tickc)

	e1 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(1)}
	bl.Log(e1)

	// Trigger an upload attempt, which will fail.
	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1})

	// Now add e2.
	e2 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(2)}
	bl.Log(e2)

	// We have not triggered any more uploads via tickc, but bl.Close should trigger an upload attempt.
	bl.Close()
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1, e2})
	expectNoMoreLogs(t, tl)
}

func TestNonTransientFailureDoesntRetry(t *testing.T) {
	// Each batch of events has 4 chances at being uploaded: one initial attempt and up to 3 retries.

	ctx := context.Background()

	// Note: err is a non-retry error.
	tl := newTestLogger(errBang)
	tickc := make(chan time.Time)
	bl := newBatchLogger(ctx, tl, tickc)

	e1 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(1)}
	bl.Log(e1)

	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e1})
	// e1 attempt #1 failed, but it won't be retried.

	tickc <- time.Time{} // trigger upload. We don't expect to receive anything from tl.requests.

	if len(tl.requests) != 0 {
		t.Errorf("events: got: %v; want: nil", tl.requests)
	}

	// Now add e2.
	e2 := &logpb.LogRequestLite_LogEventLite{EventTimeMs: proto.Int64(2)}
	bl.Log(e2)

	tickc <- time.Time{}
	expectLogCalled(t, tl, []*logpb.LogRequestLite_LogEventLite{e2})

	expectNoMoreLogs(t, tl)
	tickc <- time.Time{} // trigger upload. We don't expect to receive anything from tl.requests.
}

func TestRingBuffer(t *testing.T) {
	// the ring buffer is effectively a sliding window over a list of items, where
	// the window is moved as items are pushed into the ring buffer.
	// In this test we check that appending all the slices in the window
	// yields the same result as appending all of the slices in the ring
	// buffer.
	// The cases we test are (window over event slice is indicated with parens):
	//
	// events: [() 0, 1, 2, 3, 4, 5]      ring buffer: [nil, nil, nil]
	// events: [(0), 1, 2, 3, 4, 5]       ring buffer: [0  , nil, nil]
	// events: [(0, 1), 2, 3, 4, 5]       ring buffer: [0  , 1  , nil]
	// events: [(0, 1, 2), 3, 4, 5]       ring buffer: [0  , 1  , 2  ]
	// events: [0, (1, 2, 3), 4, 5]       ring buffer: [3  , 1  , 2  ]
	// events: [0, 1, (2, 3, 4), 5]       ring buffer: [3  , 4  , 2  ]
	// events: [0, 1, 2, (3, 4, 5)]       ring buffer: [3  , 4  , 5  ]

	events := [][]*logpb.LogRequestLite_LogEventLite{}
	for i := 0; i < numRetries*2; i++ {
		i64 := int64(i)
		e := &logpb.LogRequestLite_LogEventLite{EventTimeMs: &i64}
		events = append(events, []*logpb.LogRequestLite_LogEventLite{e})
	}

	rb := ringBuffer{}
	emptyEventSlice := make([]*logpb.LogRequestLite_LogEventLite, 0, 0)

	if got, want := rb.AppendAll(emptyEventSlice), emptyEventSlice; !reflect.DeepEqual(got, want) {
		t.Errorf("empty ring buffer AppendAll: got: %v; want: %v", got, want)
	}

	for j := 0; j < numRetries*2; j++ {
		i := j - (numRetries - 1)
		if i < 0 {
			i = 0
		}
		gotDisplaced := rb.Push(events[j])
		var wantDisplaced []*logpb.LogRequestLite_LogEventLite
		if i > 0 {
			wantDisplaced = events[i-1]
		}

		if !reflect.DeepEqual(gotDisplaced, wantDisplaced) {
			t.Errorf("ring buffer displaced: got: %v; want: %v", gotDisplaced, wantDisplaced)
		}

		got := rb.AppendAll(emptyEventSlice)
		want := appendAll(events[i : j+1])
		sort.Sort(ByTime(got))
		sort.Sort(ByTime(want))

		if !reflect.DeepEqual(got, want) {
			t.Errorf("ring buffer AppendAll (i=%v,j=%v): got: %v; want: %v", i, j, got, want)
		}
	}

}

func appendAll(events [][]*logpb.LogRequestLite_LogEventLite) []*logpb.LogRequestLite_LogEventLite {
	var result []*logpb.LogRequestLite_LogEventLite
	for _, es := range events {
		result = append(result, es...)
	}
	return result
}

type ByTime []*logpb.LogRequestLite_LogEventLite

func (bp ByTime) Len() int           { return len(bp) }
func (bp ByTime) Swap(i, j int)      { bp[i], bp[j] = bp[j], bp[i] }
func (bp ByTime) Less(i, j int) bool { return *bp[i].EventTimeMs < *bp[j].EventTimeMs }
