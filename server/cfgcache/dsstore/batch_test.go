// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dsstore

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type sizeInt int64

func (s sizeInt) size() int64 { return int64(s) }

func TestBatching(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		assert.That(t, batch[sizeInt](nil, 100), should.Match([][]sizeInt(nil)))
		assert.That(t, batch([]sizeInt{}, 100), should.Match([][]sizeInt(nil)))
	})

	t.Run("non-positive targetBatchSize puts each item in its own batch", func(t *testing.T) {
		t.Parallel()
		assert.That(t, batch([]sizeInt{10, 20}, 0), should.Match([][]sizeInt{{10}, {20}}))
		assert.That(t, batch([]sizeInt{10, 20}, -5), should.Match([][]sizeInt{{10}, {20}}))
	})

	t.Run("packs items with best-fit decreasing", func(t *testing.T) {
		t.Parallel()

		items := []sizeInt{
			90, 88, 30, 20, 80, 25, 10, 50, 60, 110,
		}

		got := batch(items, 100)
		assert.That(t, got, should.Match([][]sizeInt{
			{110},
			{90},
			{60, 50},
			{80, 30},
			{88, 20},
			{25, 10},
		}))

		// Ascending order would pack [40, 40] first and strand both 60s into 3 batches.
		assert.That(t, batch([]sizeInt{40, 40, 60, 60}, 100), should.Match([][]sizeInt{
			{60, 40},
			{60, 40},
		}))
	})

	t.Run("respects 90%, 100%, and 110% boundaries", func(t *testing.T) {
		t.Parallel()

		// >= 90% goes directly to its own batch instead of merging with 10.
		assert.That(t, batch([]sizeInt{90, 10}, 100), should.Match([][]sizeInt{
			{90},
			{10},
		}))

		// Reaching 100% evicts immediately rather than absorbing a trailing 5.
		assert.That(t, batch([]sizeInt{60, 40, 5}, 100), should.Match([][]sizeInt{
			{60, 40},
			{5},
		}))

		// Exactly 110% is allowed; > 110% is rejected.
		assert.That(t, batch([]sizeInt{60, 50}, 100), should.Match([][]sizeInt{
			{60, 50},
		}))
		assert.That(t, batch([]sizeInt{60, 51}, 100), should.Match([][]sizeInt{
			{60},
			{51},
		}))
	})

	t.Run("reorders pending batches on growth and evicts while smaller batches remain", func(t *testing.T) {
		t.Parallel()

		// 85 and 60 start pending [85, 60]; 30 skips 85 (115 > 110), joins 60 -> 90,
		// and bubbles ahead of 85 -> [90, 85]; 12 then joins 90 -> 102 and evicts,
		// leaving [85] to drain at the end.
		assert.That(t, batch([]sizeInt{85, 60, 30, 12}, 100), should.Match([][]sizeInt{
			{60, 30, 12},
			{85},
		}))
	})

	t.Run("handles zero and negative item sizes", func(t *testing.T) {
		t.Parallel()

		assert.That(t, batch([]sizeInt{60, 40, 0, 0, -5}, 100), should.Match([][]sizeInt{
			{60, 40},
			{0, 0, -5},
		}))
	})
}
