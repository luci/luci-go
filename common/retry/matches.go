// Copyright 2018 The LUCI Authors.
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

package retry

import (
	"time"

	"golang.org/x/net/context"
)

// Matches returns a Factory that wraps f, retrying only when the error matches
// fn.
//
// If an error is returned that doesn't match fn, Matches will immediately stop
// retrying.
//
// fn will be reused by each generated Factory, and must not preserve state.
func Matches(f Factory, fn func(error) bool) Factory {
	return func() Iterator {
		return &matchesIterator{
			inner: f(),
			fn:    fn,
		}
	}
}

type matchesIterator struct {
	inner Iterator
	fn    func(error) bool
}

func (it *matchesIterator) Next(c context.Context, err error) time.Duration {
	// If our error doesn't match the function, stop iterating.
	if !it.fn(err) {
		return Stop
	}
	return it.inner.Next(c, err)
}
