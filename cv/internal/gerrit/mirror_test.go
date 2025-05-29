// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMirrorIterator(t *testing.T) {
	t.Parallel()

	ftt.Run("MirrorIterator and its factory work", t, func(t *ftt.Test) {
		ctx := context.Background()
		const baseHost = "a.example.com"
		t.Run("No mirrors", func(t *ftt.Test) {
			it := newMirrorIterator(ctx)
			assert.Loosely(t, it.Empty(), should.BeFalse)
			assert.That(t, it.next()(baseHost), should.Match(baseHost))
			assert.Loosely(t, it.Empty(), should.BeTrue)
			assert.That(t, it.next()(baseHost), should.Match(baseHost))
			assert.Loosely(t, it.Empty(), should.BeTrue)
			assert.That(t, it.next()(baseHost), should.Match(baseHost))
		})
		t.Run("One mirrors", func(t *ftt.Test) {
			it := newMirrorIterator(ctx, "m1-")
			assert.Loosely(t, it.Empty(), should.BeFalse)
			assert.That(t, it.next()(baseHost), should.Match(baseHost))
			assert.Loosely(t, it.Empty(), should.BeFalse)
			assert.That(t, it.next()(baseHost), should.Match("m1-"+baseHost))
			assert.Loosely(t, it.Empty(), should.BeTrue)
			assert.That(t, it.next()(baseHost), should.Match(baseHost))
		})
		t.Run("Shuffles mirrors", func(t *ftt.Test) {
			prefixes := make([]string, 10)
			expectedHosts := make([]string, len(prefixes)+1)
			expectedHosts[0] = baseHost
			for i := range prefixes {
				// use "m" prefix such that its lexicographically after baseHost itself.
				p := fmt.Sprintf("m%d-", i)
				prefixes[i] = p
				expectedHosts[i+1] = p + baseHost
			}
			iterate := func() []string {
				var actual []string
				it := newMirrorIterator(ctx, prefixes...)
				for !it.Empty() {
					actual = append(actual, it.next()(baseHost))
				}
				return actual
			}
			act1 := iterate()
			assert.Loosely(t, act1, should.NotResemble(expectedHosts))
			act2 := iterate()
			assert.Loosely(t, act2, should.NotResemble(expectedHosts))
			assert.Loosely(t, act1, should.NotResemble(act2))

			sort.Strings(act1)
			assert.That(t, act1, should.Match(expectedHosts))
			sort.Strings(act2)
			assert.That(t, act2, should.Match(expectedHosts))
		})
		t.Run("RetryIfStale works", func(t *ftt.Test) {
			it := &MirrorIterator{"", "m1", "m2"}

			t.Run("stops when mirrors are exhausted", func(t *ftt.Test) {
				tried := 0
				err := it.RetryIfStale(func(grpc.CallOption) error {
					tried += 1
					return ErrStaleData
				})
				assert.Loosely(t, err, should.Equal(ErrStaleData))
				assert.Loosely(t, tried, should.Equal(3))
			})

			t.Run("respects returned value, unwrapping if needed", func(t *ftt.Test) {
				tried := 0
				err := it.RetryIfStale(func(grpc.CallOption) error {
					tried += 1
					if tried == 1 {
						return errors.Fmt("try #%d: %w", tried, ErrStaleData)
					}
					return errors.New("something else")
				})
				assert.ErrIsLike(t, err, "something else")
				assert.Loosely(t, tried, should.Equal(2))
				assert.That(t, (*it)[0], should.Match("m2"))
			})

			t.Run("calls at least once even if empty", func(t *ftt.Test) {
				it.Next()
				it.Next()
				it.Next()
				assert.Loosely(t, it.Empty(), should.BeTrue)
				called := false
				err := it.RetryIfStale(func(grpc.CallOption) error {
					called = true
					return nil
				})
				assert.NoErr(t, err)
				assert.Loosely(t, called, should.BeTrue)
			})
		})
	})
}
