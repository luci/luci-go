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

package parallel

import (
	"github.com/luci/luci-go/common/errors"
)

// WorkPool creates a fixed-size pool of worker goroutines. A supplied generator
// method creates task functions and passes them through to the work pool.
//
// WorkPool will use at most workers goroutines to execute the supplied tasks.
// If workers is <= 0, WorkPool will be unbounded and behave like FanOutIn.
//
// WorkPool blocks until all the generator completes and all workers have
// finished their tasks.
func WorkPool(workers int, gen func(chan<- func() error)) error {
	return multiErrorFromErrors(Run(workers, gen))
}

// FanOutIn is useful to quickly parallelize a group of tasks.
//
// You pass it a function which is expected to push simple `func() error`
// closures into the provided chan. Each function will be executed in parallel
// and their error results will be collated.
//
// The function blocks until all functions are executed, and an
// errors.MultiError is returned if one or more of your fan-out tasks failed,
// otherwise this function returns nil.
//
// This function is equivalent to WorkPool(0, gen).
func FanOutIn(gen func(chan<- func() error)) error {
	return WorkPool(0, gen)
}

// multiErrorFromErrors takes an error-channel, blocks on it, and returns
// a MultiError for any errors pushed to it over the channel, or nil if
// all the errors were nil.
func multiErrorFromErrors(ch <-chan error) error {
	if ch == nil {
		return nil
	}
	ret := errors.MultiError(nil)
	for e := range ch {
		if e == nil {
			continue
		}
		ret = append(ret, e)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
