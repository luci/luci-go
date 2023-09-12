// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package swarming

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// ErrAllPending is returned by GetAnyCompleted in Poll mode if all tasks are
// pending.
var ErrAllPending = errors.New("all requested tasks are still pending")

// BlockMode is passed to Get* family of functions and indicates if they should
// block or not.
type BlockMode bool

const (
	// Poll means to fetch the current task status, do not wait for completion.
	Poll BlockMode = false
	// Wait means to wait until the task completes and return the final status.
	Wait BlockMode = true
)

// GetOne returns the status of a single task, optionally waiting for it to
// complete first.
//
// If `mode` is Poll, will just query the task status once, otherwise (if `mode`
// is Wait), will block until the task completes (successfully or not) or the
// context expires.
//
// On success returns task's status and a nil error. If `mode` is Poll, it might
// be a non-final status (i.e. PENDING or RUNNING). If `mode` is Wait, this will
// always be a final status.
//
// Exits early if the context expires or if RPCs to Swarming fail. Returns a
// non-nil error in this case. It will either be a gRPC error or a context
// error.
//
// Panics if `taskID` is an empty string.
func GetOne(ctx context.Context, client Client, taskID string, withPerf bool, mode BlockMode) (*swarmingv2.TaskResultResponse, error) {
	if taskID == "" {
		panic("taskID should not be empty")
	}

	startedTime := clock.Now(ctx)
	for {
		res, err := client.TaskResult(ctx, taskID, withPerf)
		switch status.Code(err) {
		case codes.OK:
			if mode == Poll || final(res) {
				// Check this API invariant, since the caller likely relies on TaskId in
				// the response to match it to the original request.
				if res.TaskId != taskID {
					return nil, status.Errorf(codes.FailedPrecondition, "expecting TaskResultResponse with task ID %q, but got %q", taskID, res.TaskId)
				}
				return res, nil
			}
		case codes.DeadlineExceeded, codes.Canceled:
			// This can be a server side or a client side error. If it is a client
			// side error (e.g. our context has expired), we'll discover it soon
			// enough when we try to sleep. If it is a server side error, we'll retry.
		default:
			// Either a fatal RPC error or a transient error after many retries.
			return nil, err
		}

		// Start with a 1 second delay and for each 30 seconds of waiting add
		// another second until hitting the ceiling.
		delay := time.Second + clock.Since(ctx, startedTime)/30
		if delay >= 15*time.Second {
			delay = 15 * time.Second
		}

		// Randomize delay to avoid visiting all pending tasks in a lockstep when
		// called by GetAll or GetAnyCompleted.
		delay += time.Duration(mathrand.Int63n(ctx, int64(delay/3)))

		// Sleep, but be ready for the context to expire while we sleep.
		clock.Sleep(clock.Tag(ctx, "wait-"+taskID), delay)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

// GetAll queries status of all given tasks, optionally waiting for them to
// complete.
//
// This is a batch variant of GetOne which essentially runs multiple queries
// concurrently and calls the callback with result of each query as soon as it
// is available. The advantage over calling GetOne in parallel is that GetAll
// may schedule TaskResult calls in a more smart way to avoid QPS spikes (but
// it currently doesn't).
//
// The callback may be called concurrently. It will always be called exactly
// `len(taskIDs)` times, once per each task. The callback should not block. If
// it needs to do further processing, it should do it asynchronously.
//
// Panics if `taskIDs` is empty or any of given task IDs is an empty string.
func GetAll(ctx context.Context, client Client, taskIDs []string, withPerf bool, mode BlockMode, sink func(taskID string, res *swarmingv2.TaskResultResponse, err error)) {
	if len(taskIDs) == 0 {
		panic("need at least one task to wait for")
	}

	// TODO(vadimsh): This implementation is not really worth a separate function
	// right now, but GetAll abstraction by itself is still useful, since it
	// allows to do something smarter in the future (e.g. limit TaskResult QPS).

	wg := sync.WaitGroup{}
	wg.Add(len(taskIDs))
	defer wg.Wait()

	for _, taskID := range taskIDs {
		taskID := taskID
		go func() {
			defer wg.Done()
			res, err := GetOne(ctx, client, taskID, withPerf, mode)
			sink(taskID, res, err)
		}()
	}
}

// GetAnyCompleted queries status of given tasks and returns some completed one.
//
// If `mode` is Poll, will just examine the current state and pick a completed
// task (if any) randomly. If all tasks are still pending, returns ErrAllPending
// error.
//
// If `mode` is Wait will block until any of the given tasks completes and will
// returns its status. If multiple tasks complete at the same time, returns any
// one of them randomly.
//
// Exits early if the context expires or if Swarming replies with a fatal error
// on any of sent RPCs. When this happens returns a task ID that triggered
// the error, nil task result and the non-nil error itself. It will either be
// a gRPC error or a context error.
//
// Panics if `taskIDs` is empty or any of given task IDs is an empty string.
func GetAnyCompleted(ctx context.Context, client Client, taskIDs []string, withPerf bool, mode BlockMode) (taskID string, res *swarmingv2.TaskResultResponse, err error) {
	ctx, done := context.WithCancel(ctx)
	defer done()

	m := sync.Mutex{}

	GetAll(ctx, client, taskIDs, withPerf, mode, func(gotTaskID string, gotRes *swarmingv2.TaskResultResponse, gotErr error) {
		if gotErr != nil || final(gotRes) {
			m.Lock()
			defer m.Unlock()
			if taskID == "" {
				taskID = gotTaskID
				res = gotRes
				err = gotErr
				done() // got one result, abort waiting for the rest
			}
		}
	})

	// This means we visited all tasks and none of them is complete.
	if taskID == "" {
		err = ErrAllPending
	}

	return
}

// final is true if the task is in some final state.
func final(res *swarmingv2.TaskResultResponse) bool {
	return res.State != swarmingv2.TaskState_PENDING && res.State != swarmingv2.TaskState_RUNNING
}
