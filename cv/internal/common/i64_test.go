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

package common

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestI64s(t *testing.T) {
	t.Parallel()

	ftt.Run("UniqueSorted", t, func(t *ftt.Test) {
		v := []int64{7, 6, 3, 1, 3, 4, 9, 2, 1, 5, 8, 8, 8, 4, 9}
		v1 := UniqueSorted(v)
		assert.That(t, v1, should.Match([]int64{1, 2, 3, 4, 5, 6, 7, 8, 9}))
		assert.That(t, v[:len(v1)], should.Match(v1)) // reuses space

		v = []int64{1, 2, 2, 3, 4, 5, 5}
		v1 = UniqueSorted(v)
		assert.That(t, v1, should.Match([]int64{1, 2, 3, 4, 5}))
		assert.That(t, v[:len(v1)], should.Match(v1)) // reuses space
	})

	ftt.Run("DifferenceSorted", t, func(t *ftt.Test) {
		all := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
		odd := []int64{1, 3, 5, 7, 9, 11}
		div3 := []int64{3, 6, 9}

		assert.Loosely(t, DifferenceSorted(odd, all), should.BeEmpty)
		assert.Loosely(t, DifferenceSorted(div3, all), should.BeEmpty)

		assert.That(t, DifferenceSorted(all, odd), should.Match([]int64{2, 4, 6, 8, 10}))
		assert.That(t, DifferenceSorted(all, div3), should.Match([]int64{1, 2, 4, 5, 7, 8, 10, 11}))

		assert.That(t, DifferenceSorted(odd, div3), should.Match([]int64{1, 5, 7, 11}))
		assert.That(t, DifferenceSorted(div3, odd), should.Match([]int64{6}))
	})

	ftt.Run("UnionSorted", t, func(t *ftt.Test) {
		all := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9}
		odd := []int64{1, 3, 5, 7, 9}
		div3 := []int64{3, 6, 9}

		assert.That(t, UnionSorted(odd, all), should.Match(all))
		assert.That(t, UnionSorted(all, div3), should.Match(all))
		assert.That(t, UnionSorted(all, all), should.Match(all))
		assert.That(t, UnionSorted(all, nil), should.Match(all))
		assert.That(t, UnionSorted(nil, all), should.Match(all))

		assert.That(t, UnionSorted(odd, div3), should.Match([]int64{1, 3, 5, 6, 7, 9}))
		assert.That(t, UnionSorted(div3, odd), should.Match([]int64{1, 3, 5, 6, 7, 9}))
	})
}
