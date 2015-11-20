// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package promise

import (
	"errors"

	"golang.org/x/net/context"
)

var (
	// ErrNoData is an error returned when a request completes without available data.
	ErrNoData = errors.New("promise: No Data")
)

// Promise is a promise structure with goroutine-safe methods that is
// responsible for owning a single piece of data. Promises have multple readers
// and a single writer.
//
// Readers will retrieve the Promise's data via Get(). If the data has not been
// populated, the reader will block pending the data. Once the data has been
// delivered, all readers will unblock and receive a reference to the Promise's
// data.
type Promise interface {
	// Get returns the promise's value. If the value isn't set, Get will block until
	// the value is available, following the Context's timeout parameters.
	//
	// If the value is available, it will be returned with its error status. If the
	// context times out or is cancelled, the appropriate context error will be
	// returned.
	Get(context.Context) (interface{}, error)

	// Peek returns the promise's current value. If the value isn't set, Peek will
	// return immediately with ErrNoData.
	Peek() (interface{}, error)
}

// promiseImpl is an implementation of the Promise interface.
type promiseImpl struct {
	signalC chan struct{} // Channel whose closing signals that the data is available.

	data interface{} // The Promise's data.
	err  error       // The error status.
}

var _ Promise = (*promiseImpl)(nil)

// New instantiates a new, empty Promise instance. The Promise's value will be
// the value returned by the supplied generator function.
//
// The generator will be invoked immediately in its own goroutine.
func New(gen func() (interface{}, error)) Promise {
	p := promiseImpl{
		signalC: make(chan struct{}),
	}

	// Execute our generator function in a separate goroutine.
	go func() {
		defer close(p.signalC)
		p.data, p.err = gen()
	}()

	return &p
}

func (p *promiseImpl) Get(c context.Context) (interface{}, error) {
	// Block until at least one of these conditions is satisfied. If both are,
	// "select" will choose one pseudo-randomly.
	select {
	case <-p.signalC:
		return p.data, p.err

	case <-c.Done():
		// Make sure we don't actually have data.
		select {
		case <-p.signalC:
			return p.data, p.err

		default:
			break
		}
		return nil, c.Err()
	}
}

// Peek returns the promise's current value. If the value isn't set, Peek will
// return immediately with ErrNoData.
func (p *promiseImpl) Peek() (interface{}, error) {
	select {
	case <-p.signalC:
		return p.data, p.err

	default:
		return nil, ErrNoData
	}
}
