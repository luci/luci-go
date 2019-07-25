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

// Package errctx provides implementations of context.Context that allow for
// cancellation or deadline expiration with custom error messages.
package errctx

import (
	"context"
	"sync"
	"time"
)

// cancelContext is an implementation of context.Context, in which the context
// can be cancelled with a custom error.
type cancelContext struct {
	parent context.Context

	cancelOnce sync.Once
	done       chan struct{}
	errs       chan error
	err        error
}

// Deadline implements context.Context.
func (c *cancelContext) Deadline() (time.Time, bool) {
	return c.parent.Deadline()
}

// Done implements context.Context.
func (c *cancelContext) Done() <-chan struct{} {
	return c.done
}

// Err implements context.Context.
func (c *cancelContext) Err() error {
	return c.err
}

// Value implements context.Context.
func (c *cancelContext) Value(key interface{}) interface{} {
	return c.parent.Value(key)
}

// cancel cancels the context with the given error.
func (c *cancelContext) cancel(err error) {
	c.cancelOnce.Do(func() {
		c.errs <- err
		close(c.errs)
	})
}

// propagate launches a goroutine that propogates context cancellation from
// parent context.
func (c *cancelContext) propagate() {
	go func() {
		select {
		case <-c.parent.Done():
			c.err = c.parent.Err()
		case err := <-c.errs:
			c.err = err
		}
		close(c.done)

		// ensure that c.errs is closed and fully consumed.
		c.cancel(nil)
		for range c.errs {
		}
	}()
}

func newCancelContext(parent context.Context) *cancelContext {
	c := &cancelContext{
		parent: parent,
		done:   make(chan struct{}),
		errs:   make(chan error, 1),
	}
	c.propagate()
	return c
}

// WithCancel returns a child context with a cancellation function that accepts
// custom errors.
func WithCancel(parent context.Context) (context.Context, func(error)) {
	c := newCancelContext(parent)
	return c, c.cancel
}
