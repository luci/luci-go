// Copyright 2015 The LUCI Authors.
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
