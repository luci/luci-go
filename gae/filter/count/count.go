// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package count contains 'counter' filters for all the gae services. This
// serves as a set of simple example filters, and also enables other filters
// to test to see if certain underlying APIs are called when they should be
// (e.g. for the datastore mcache filter, for example).
package count

import (
	"sync/atomic"
)

// Entry is a success/fail pair for a single API method. It's returned
// by the Counter interface.
type Entry struct {
	Successes int64
	Errors    int64
}

// Total is a convenience function for getting the total number of calls to
// this API. It's Successes+Errors.
func (e Entry) Total() int64 { return e.Successes + e.Errors }

func (e *Entry) up(errs ...error) error {
	err := error(nil)
	if len(errs) > 0 {
		err = errs[0]
	}
	if err == nil {
		atomic.AddInt64(&e.Successes, 1)
	} else {
		atomic.AddInt64(&e.Errors, 1)
	}
	return err
}
