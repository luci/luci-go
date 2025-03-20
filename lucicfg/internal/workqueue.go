// Copyright 2025 The LUCI Authors.
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

package internal

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// WorkQueue works on a queue of work items in parallel until it is empty.
//
// Work items can be enqueued from the work callback. Currently has no limit
// on concurrency (it can be added later if becomes necessary).
type WorkQueue[T any] struct {
	// Note: right now WorkQueue is essentially just errgroup.Group. But if we
	// ever want to add a concurrency limit, we won't be able to use errgroup
	// directly (because when it is limited, its Go(...) becomes blocking, which
	// will lead to deadlocks if new work is enqueued from a worker callback). We
	// introduce WorkQueue API to clearly indicate where we significantly rely on
	// non-blocking behavior of WorkQueue.Submit.
	eg   *errgroup.Group
	ctx  context.Context
	exec func(T) error
}

// NewWorkQueue returns a new unlimited work queue.
//
// The given context will be canceled as soon as any of the work callbacks
// returns an error.
func NewWorkQueue[T any](ctx context.Context) (*WorkQueue[T], context.Context) {
	eg, ctx := errgroup.WithContext(ctx)
	return &WorkQueue[T]{eg: eg, ctx: ctx}, ctx
}

// Launch launches the work queue workers.
func (wq *WorkQueue[T]) Launch(exec func(T) error) {
	wq.exec = exec
}

// Submit schedules this work item to be executed asynchronously.
//
// If processing this items finishes with an error, the work queue context will
// be canceled and this error will be returned from Wait().
func (wq *WorkQueue[T]) Submit(work T) {
	wq.eg.Go(func() error {
		if err := wq.ctx.Err(); err != nil {
			return err
		}
		return wq.exec(work)
	})
}

// Wait waits for the queue to become idle.
//
// If any of the worker callbacks fails, returns the first such observed error.
// When this happens, the work queue context is canceled and all remaining
// pending work is discarded (due to inherent raciness of this process, the
// worker callback still can be called for some pending work item with the
// context expiring midway through execution).
func (wq *WorkQueue[T]) Wait() error {
	return wq.eg.Wait()
}
