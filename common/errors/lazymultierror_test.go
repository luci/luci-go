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

package errors

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLazyMultiError(t *testing.T) {
	t.Parallel()

	ftt.Run("Test LazyMultiError", t, func(t *ftt.Test) {
		lme := NewLazyMultiError(10)
		assert.Loosely(t, lme.Get(), should.BeNil)

		e := errors.New("sup")
		lme.Assign(6, e)
		assert.Loosely(t, lme.Get(), should.Match(
			[]error{nil, nil, nil, nil, nil, nil, e, nil, nil, nil},
			cmpopts.EquateErrors()))

		lme.Assign(2, e)
		assert.Loosely(t, lme.Get(), should.Match(
			[]error{nil, nil, e, nil, nil, nil, e, nil, nil, nil},
			cmpopts.EquateErrors()))

		assert.Loosely(t, func() { lme.Assign(20, e) }, should.Panic)

		t.Run("Try to freak out the race detector", func(t *ftt.Test) {
			lme := NewLazyMultiError(64)
			t.Run("all nils", func(t *ftt.Test) {
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						lme.Assign(i, nil)
						wg.Done()
					}(i)
				}
				wg.Wait()
				assert.Loosely(t, lme.Get(), should.BeNil)
			})
			t.Run("every other", func(t *ftt.Test) {
				wow := errors.New("wow")
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						e := error(nil)
						if i&1 == 1 {
							e = wow
						}
						lme.Assign(i, e)
						wg.Done()
					}(i)
				}
				wg.Wait()
				me := make([]error, 64)
				for i := range me {
					if i&1 == 1 {
						me[i] = wow
					}
				}
				assert.Loosely(t, lme.Get(), should.Match(me, cmpopts.EquateErrors()))
			})
			t.Run("all", func(t *ftt.Test) {
				wow := errors.New("wow")
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						lme.Assign(i, wow)
						wg.Done()
					}(i)
				}
				wg.Wait()
				me := make([]error, 64)
				for i := range me {
					me[i] = wow
				}
				assert.Loosely(t, lme.Get(), should.Match(me, cmpopts.EquateErrors()))
			})
		})
	})
}
