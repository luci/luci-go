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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"

	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// WaitMode is passed to Get* functions and indicates how they should block.
type WaitMode int

const (
	// NoWait means to fetch the current task status, do not wait for completion.
	NoWait WaitMode = 0

	// WaitAny means to wait until at least one task completes and then return
	// the current state of all tasks: at least one of them will be completed, but
	// possibly more (if multiple tasks complete at the same time).
	WaitAny WaitMode = 1

	// WaitAll means to wait until all tasks complete.
	WaitAll WaitMode = 2
)

// GetOne returns the status of a single task, optionally waiting for it to
// complete first.
//
// If `mode` is NoWait, will just query the task status once, otherwise (if
// `mode` is WaitAny or WaitAll), will block until the task completes
// (successfully or not) or the context expires.
//
// On success returns task's status and a nil error. If `mode` is NoWait, it
// might be a non-final status (i.e. PENDING or RUNNING). In other modes this
// will always be a final status.
//
// Exits early if the context expires or if RPCs to Swarming fail with a fatal
// error code. Returns a non-nil error in this case. It will either be a gRPC
// error or a context error. If an error happened after the function fetched
// task's state at least once, returns the latest fetched state (which will be
// either PENDING or RUNNING) as well.
//
// Retries on transient errors until the context expires.
//
// Panics if `taskID` is an empty string.
func GetOne(ctx context.Context, client Client, taskID string, fields *TaskResultFields, mode WaitMode) (*swarmingpb.TaskResultResponse, error) {
	if taskID == "" {
		panic("taskID should not be empty")
	}

	startTime := clock.Now(ctx)
	var lastKnown *swarmingpb.TaskResultResponse

	for {
		switch res, err := client.TaskResult(ctx, taskID, fields); {
		case err != nil && ctx.Err() != nil:
			// Prefer local context errors over gRPC's DEADLINE_EXCEEDED error.
			return lastKnown, ctx.Err()
		case err != nil && isFatalError(err):
			// Abort only on fatal errors such as PERMISSION_DENIED.
			return lastKnown, err
		case err == nil:
			lastKnown = res
			if mode == NoWait || final(res.State) {
				return res, nil
			}
		}
		// If the task is still not final or we got a transient error, retry later.
		if err := sleep(ctx, startTime, 1); err != nil {
			return lastKnown, err
		}
	}
}

// GetMany queries status of all given tasks, optionally waiting for them to
// complete.
//
// This is a variant of GetOne which uses batch RPCs to reduce overhead. It
// calls the callback with result of each task as soon as it is available.
// See GetOne doc for the semantics of values passed to the callback.
//
// If `mode` NoWait, will fetch the current state of all tasks and pass them to
// the callback. If `mode` is WaitAny, will block until at least one task
// completes, and then pass states of all tasks to the callback. If `mode` is
// WaitAll, will block until all tasks complete, calling the callback whenever
// it detects any new completions.
//
// The callback will always be called exactly `len(taskIDs)` times, once per
// each task, in undefined order. The callback should not block. If it needs to
// do further processing, it should do it asynchronously.
//
// Retries on transient errors until the context expires.
//
// Panics if `taskIDs` is empty or any of given task IDs is an empty string.
func GetMany(ctx context.Context, client Client, taskIDs []string, fields *TaskResultFields, mode WaitMode, sink func(taskID string, res *swarmingpb.TaskResultResponse, err error)) {
	if len(taskIDs) == 0 {
		panic("need at least one task to wait for")
	}

	startTime := clock.Now(ctx)

	// Current state. Keys are task IDs, values are:
	//   * ResultOrErr{Result: nil, Err: nil} if haven't got any information yet.
	//   * ResultOrErr{Result: ..., Err: nil} if the task is not final yet.
	//   * ResultOrErr{Result: nil, Err: ...} if failed to fetch the task result.
	awaiting := make(map[string]ResultOrErr, len(taskIDs))
	for _, taskID := range taskIDs {
		awaiting[taskID] = ResultOrErr{}
	}

	// Sends all remaining results to the sink, optionally setting the error.
	flushRemaining := func(err error) {
		for taskID, res := range awaiting {
			if err != nil {
				sink(taskID, res.Result, err)
			} else {
				if res.Result == nil && res.Err == nil {
					panic("should not be possible")
				}
				sink(taskID, res.Result, res.Err)
			}
			delete(awaiting, taskID)
		}
	}

	awaitingIDs := make([]string, 0, len(taskIDs))
	for {
		// Collect task IDs we still waiting for. Reuse the slice across iterations.
		awaitingIDs = awaitingIDs[:0]
		for taskID := range awaiting {
			awaitingIDs = append(awaitingIDs, taskID)
		}

		// Ask the backend for status of all tasks.
		switch res, err := client.TaskResults(ctx, awaitingIDs, fields); {
		case err != nil && ctx.Err() != nil:
			// Prefer local context errors over gRPC's DEADLINE_EXCEEDED error.
			flushRemaining(ctx.Err())
			return
		case err != nil && isFatalError(err):
			// Abort only on fatal errors such as PERMISSION_DENIED.
			flushRemaining(err)
			return
		case err == nil:
			// Update last known task results and errors.
			for i, taskID := range awaitingIDs {
				lastKnown := awaiting[taskID]
				switch fetched := res[i]; {
				case fetched.Result != nil:
					lastKnown.Result = fetched.Result
				case fetched.Err != nil:
					lastKnown.Err = fetched.Err
				default:
					panic("impossible empty result in TaskResults response")
				}
				awaiting[taskID] = lastKnown
			}
			// If not asked to wait at all, we are done.
			if mode == NoWait {
				flushRemaining(nil)
				return
			}
		}

		// Flush completed or erroneous tasks.
		completed := 0
		for taskID, res := range awaiting {
			done := res.Result != nil && final(res.Result.State)
			if res.Err != nil || done {
				sink(taskID, res.Result, res.Err)
				delete(awaiting, taskID)
				if done {
					completed++
				}
			}
		}

		switch {
		case len(awaiting) == 0:
			// If there's nothing to wait for, we are done.
			return
		case mode == WaitAny && completed != 0:
			// If were waiting for at least one task and got it, we are done.
			flushRemaining(nil)
			return
		default:
			// If still have tasks or had a transient error, sleep and retry.
			if err := sleep(ctx, startTime, len(awaiting)); err != nil {
				flushRemaining(err)
				return
			}
		}
	}
}

// isFatalError returns true if this gRPC client error is fatal.
func isFatalError(err error) bool {
	switch status.Code(err) {
	case codes.OK,
		codes.Internal,
		codes.Unknown,
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.Canceled:
		return false
	default:
		return true
	}
}

// final is true if the task is in some final state.
func final(state swarmingpb.TaskState) bool {
	return state != swarmingpb.TaskState_PENDING && state != swarmingpb.TaskState_RUNNING
}

// sleep sleeps a bit between retry loop iterations.
func sleep(ctx context.Context, startTime time.Time, taskCount int) error {
	// Start with a 1 second delay and for each 30 seconds of waiting add
	// another second until hitting the ceiling.
	delay := time.Second + clock.Since(ctx, startTime)/30
	if delay >= 15*time.Second {
		delay = 15 * time.Second
	}

	// Randomize delay to avoid sending RPCs in a lockstep.
	delay += time.Duration(mathrand.Int63n(ctx, int64(delay/3)))

	// This will abort early if the context expires.
	logging.Debugf(ctx, "Sleeping %s, waiting for %d tasks", delay.Round(100*time.Millisecond), taskCount)
	clock.Sleep(ctx, delay)
	return ctx.Err()
}
