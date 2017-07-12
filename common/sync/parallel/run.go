// Copyright 2016 The LUCI Authors.
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

// Run executes a generator function, dispatching each generated task to the
// Runner. Run returns immediately with an error channel that can be used to
// reap the results of those tasks.
//
// The returned error channel must be consumed, or it can block additional
// functions from being run from gen. A common consumption function is
// errors.MultiErrorFromErrors, which will buffer all non-nil errors into an
// errors.MultiError. Other functions to consider are Must and Ignore (in this
// package).
//
// Note that there is no association between error channel's error order and
// the generated task order. However, the channel will return exactly one error
// result for each generated task.
//
// If workers is <= 0, it will be unbounded; otherwise, a pool of at most
// workers sustained goroutines will be used to execute the task.
func Run(workers int, gen func(chan<- func() error)) <-chan error {
	r := Runner{
		Maximum:   workers,
		Sustained: workers,
	}
	return r.runThen(gen, r.Close)
}
