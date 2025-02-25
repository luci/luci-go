// Copyright 2024 The LUCI Authors.
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

package retention

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChunk(t *testing.T) {
	t.Parallel()

	ftt.Run("Chunk", t, func(t *ftt.Test) {
		assert.Loosely(t, chunk([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 3), should.Match([][]int{
			{1, 2, 3},
			{4, 5, 6},
			{7, 8, 9},
			{10},
		}))
		assert.Loosely(t, chunk([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 2), should.Match([][]int{
			{1, 2},
			{3, 4},
			{5, 6},
			{7, 8},
			{9, 10},
		}))
		assert.Loosely(t, chunk([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 11), should.Match([][]int{
			{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		}))
	})
}
