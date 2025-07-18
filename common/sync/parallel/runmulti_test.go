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
	"context"
	"sort"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRunMulti(t *testing.T) {
	t.Parallel()

	ftt.Run(`A RunMulti operation with two workers can be nested without deadlock.`, t, func(t *ftt.Test) {
		const n = 2
		const inner = 128

		// This will hand out "n" tokens, then close "tokensOutC".
		tokenC := make(chan struct{})
		tokensOutC := make(chan struct{})
		go func() {
			defer close(tokensOutC)

			for range n {
				tokenC <- struct{}{}
			}
		}()

		err := RunMulti(context.Background(), n, func(mr MultiRunner) error {
			return mr.RunMulti(func(workC chan<- func() error) {
				// Dispatch n top-level dispatchers and block until they are both
				// executed. This will consume the total number of workers. In a normal
				// Runner, this would prevent the top-level dispatchers' dispatched
				// routines from running.
				for i := range n {
					workC <- func() error {
						// Take one token.
						<-tokenC

						// Wait until all of the tokens have been taken.
						<-tokensOutC

						// Dispatch a bunch of sub-work.
						return mr.RunMulti(func(workC chan<- func() error) {
							for j := range inner {
								index := (i * inner) + j
								workC <- func() error { return numberError(index) }
							}
						})
					}
				}
			})
		})

		// Flatten our "n" top-level MultiErrors together.
		assert.Loosely(t, err, should.HaveType[errors.MultiError])
		aggregateErr := make(errors.MultiError, 0, (n * inner))
		for _, ierr := range err.(errors.MultiError) {
			assert.Loosely(t, ierr, should.HaveType[errors.MultiError])
			aggregateErr = append(aggregateErr, ierr.(errors.MultiError)...)
		}
		assert.Loosely(t, aggregateErr, should.HaveLength((n * inner)))

		// Make sure all of the error values that we expect are present.
		actual := make([]int, len(aggregateErr))
		expected := make([]int, len(aggregateErr))
		for i := range aggregateErr {
			actual[i] = int(aggregateErr[i].(numberError))
			expected[i] = i
		}
		sort.Ints(actual)
		assert.Loosely(t, actual, should.Match(expected))
	})

	ftt.Run(`A RunMulti operation will stop executing jobs if its Context is canceled.`, t, func(t *ftt.Test) {
		const n = 128
		const cancelPoint = 16

		c, cancelFunc := context.WithCancel(context.Background())
		err := RunMulti(c, 1, func(mr MultiRunner) error {
			return mr.RunMulti(func(workC chan<- func() error) {
				for i := range n {
					if i == cancelPoint {
						// This and all future work should not be dispatched. Our previous
						// work item *may* execute depending on whether it was dispatched
						// before or after the cancel was processed.
						cancelFunc()
					}

					workC <- func() error { return nil }
				}
			})
		})

		// We should have somewhere between (n-cancelPoint-1) and (n-cancelPoint)
		// context errors.
		assert.Loosely(t, err, should.HaveType[errors.MultiError])
		assert.Loosely(t, len(err.(errors.MultiError)), should.BeBetweenOrEqual(n-cancelPoint-1, n-cancelPoint))
	})

	ftt.Run(`A RunMulti operation with no worker limit will not be constrained.`, t, func(t *ftt.Test) {
		const n = 128

		// This will hand out "n" tokens, then close "tokensOutC".
		tokenC := make(chan struct{})
		tokensOutC := make(chan struct{})
		go func() {
			defer close(tokensOutC)

			for range n {
				tokenC <- struct{}{}
			}
		}()

		err := RunMulti(context.Background(), 0, func(mr MultiRunner) error {
			// This will deadlock if all n workers can't run simultaneously.
			return mr.RunMulti(func(workC chan<- func() error) {
				for range n {
					workC <- func() error {
						<-tokenC
						<-tokensOutC
						return nil
					}
				}
			})
		})
		assert.Loosely(t, err, should.BeNil)
	})
}
