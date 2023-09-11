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
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
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
// Exits early if the context expires or if Swarming replies with a fatal RPC
// error. Returns a non-nil error in this case. It will either be a gRPC error
// or a context error.
//
// In either BlockMode retries on transient errors until the context expiration.
func GetOne(ctx context.Context, client Client, taskID string, withPerf bool, mode BlockMode) (*swarmingv2.TaskResultResponse, error) {
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
		case codes.Internal, codes.Unknown, codes.Unavailable:
			// Most likely a transient error, try again later. Note that gRPC client
			// likely implements its own retry loop already, but we don't want to
			// rely on its configuration: GetOne promises to retry transient errors
			// "forever" (until the context expires).
		case codes.DeadlineExceeded, codes.Canceled:
			// This can be a server side or a client side error. If it is a client
			// side error (e.g. our context has expired), we'll discover it soon
			// enough when we try to sleep. If it is a server side error, we'll retry.
		default:
			// A fatal RPC error.
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
// Calls the given callback as soon as any of given tasks' status is available,
// perhaps waiting for tasks to complete first. If `mode` is Poll, it might be
// a non-final status (i.e. PENDING or RUNNING). If `mode` is Wait, this will
// always be a final status.
//
// The callback may be called concurrently. The callback should not block. If it
// needs to do further processing, it should do it asynchronously.
//
// Exits with no error if successfully queried statuses of all tasks (perhaps
// after waiting for them to complete).
//
// Exits early if the context expires or if Swarming replies with a fatal RPC
// error on any of sent RPCs. As soon as this happens all pending queries are
// canceled and the callback is not called for them. Returns a non-nil error in
// this case. It will either be a gRPC error or a context error.
//
// In either BlockMode retries on transient errors until the context expiration.
//
// Panics if `taskIDs` is empty.
func GetAll(ctx context.Context, client Client, taskIDs []string, withPerf bool, mode BlockMode, sink func(*swarmingv2.TaskResultResponse)) error {
	if len(taskIDs) == 0 {
		panic("need at least one task to wait for")
	}

	eg, gctx := errgroup.WithContext(ctx)

	var reported int32
	for _, taskID := range taskIDs {
		taskID := taskID
		eg.Go(func() error {
			res, err := GetOne(gctx, client, taskID, withPerf, mode)
			if err == nil && gctx.Err() == nil {
				sink(res)
				atomic.AddInt32(&reported, 1)
			}
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		// If reported all tasks, then ignore any errors here. They can
		// theoretically happen due to context cancellation racing with the
		// successful completion. This should be super rare, but the check isn't
		// difficult either.
		if atomic.LoadInt32(&reported) == int32(len(taskIDs)) {
			return nil
		}
		// Return a cleaner error if the root context expires (instead of
		// "aborted RPC call" error).
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	return nil
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
// on any of sent RPCs. Returns nil result and non-nil error in this case. It
// will either be a gRPC error or a context error.
//
// In either BlockMode retries on transient errors until the context expiration.
//
// Panics if `taskIDs` is empty.
func GetAnyCompleted(ctx context.Context, client Client, taskIDs []string, withPerf bool, mode BlockMode) (*swarmingv2.TaskResultResponse, error) {
	ctx, done := context.WithCancel(ctx)
	defer done()

	var success *swarmingv2.TaskResultResponse
	err := GetAll(ctx, client, taskIDs, withPerf, mode, func(res *swarmingv2.TaskResultResponse) {
		if final(res) {
			success = res
			done() // got one result, abort waiting for the rest
		}
	})

	switch {
	case success != nil:
		// If got a result, ignore any potential error (which most likely be
		// "context was canceled", since we canceled the context ourselves).
		return success, nil
	case err == nil:
		// This means we visited all tasks and none of them is complete.
		return nil, ErrAllPending
	default:
		// Either a gRPC or a context error.
		return nil, err
	}
}

// final is true if the task is in some final state.
func final(res *swarmingv2.TaskResultResponse) bool {
	return res.State != swarmingv2.TaskState_PENDING && res.State != swarmingv2.TaskState_RUNNING
}
