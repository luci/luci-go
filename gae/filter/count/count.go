// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package count contains 'counter' filters for all the gae services. This
// serves as a set of simple example filters, and also enables other filters
// to test to see if certain underlying APIs are called when they should be
// (e.g. for the datastore mcache filter, for example).
package count

import (
	"fmt"
	"sync/atomic"
)

type counter struct {
	value int32
}

func (c *counter) increment() {
	atomic.AddInt32(&c.value, 1)
}

func (c *counter) get() int {
	return int(atomic.LoadInt32(&c.value))
}

// Entry is a success/fail pair for a single API method. It's returned
// by the Counter interface.
type Entry struct {
	successes counter
	errors    counter
}

func (e *Entry) String() string {
	return fmt.Sprintf("{Successes:%d, Errors:%d}", e.Successes(), e.Errors())
}

// Total is a convenience function for getting the total number of calls to
// this API. It's Successes+Errors.
func (e *Entry) Total() int64 { return int64(e.Successes()) + int64(e.Errors()) }

// Successes returns the number of successful invocations for this Entry.
func (e *Entry) Successes() int {
	return e.successes.get()
}

// Errors returns the number of unsuccessful invocations for this Entry.
func (e *Entry) Errors() int {
	return e.errors.get()
}

func (e *Entry) up(errs ...error) error {
	err := error(nil)
	if len(errs) > 0 {
		err = errs[0]
	}
	if err == nil {
		e.successes.increment()
	} else {
		e.errors.increment()
	}
	return err
}
