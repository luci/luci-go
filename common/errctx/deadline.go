// Copyright 2019 The LUCI Authors.
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

package errctx

import (
	"context"
	"time"
)

// deadlineContext is an implementation of context.Context, in which the context
// is cancelled with a custom error after the deadline expires.
type deadlineContext struct {
	cancelContext *cancelContext
	deadline      time.Time
}

// Deadline implements context.Context.
func (c *deadlineContext) Deadline() (time.Time, bool) {
	return c.deadline, true
}

// Done implements context.Context.
func (c *deadlineContext) Done() <-chan struct{} {
	return c.cancelContext.Done()
}

// Err implements context.Context.
func (c *deadlineContext) Err() error {
	return c.cancelContext.Err()
}

// Value implements context.Context.
func (c *deadlineContext) Value(key interface{}) interface{} {
	return c.cancelContext.Value(key)
}

func newDeadlineContext(parent context.Context, deadline time.Time) *deadlineContext {
	parentDeadline, ok := parent.Deadline()
	if ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return &deadlineContext{
		cancelContext: newCancelContext(parent),
		deadline:      deadline,
	}
}

// WithDeadline returns a child context that expires with a custom error after
// the supplied deadline, and a cancellation function that accepts custom errors.
func WithDeadline(parent context.Context, deadline time.Time, err error) (context.Context, func(error)) {
	c := newDeadlineContext(parent, deadline)
	timer := time.NewTimer(deadline.Sub(time.Now()))
	go func() {
		select {
		case <-c.Done():
			// Do nothing.
		case <-timer.C:
			c.cancelContext.cancel(err)
		}
		timer.Stop()
	}()
	return c, c.cancelContext.cancel
}

// WithTimeout returns a child context that expires with a custom error after
// the duration expires, and a cancellation function that accepts custom errors.
func WithTimeout(parent context.Context, duration time.Duration, err error) (context.Context, func(error)) {
	return WithDeadline(parent, time.Now().Add(duration), err)
}
