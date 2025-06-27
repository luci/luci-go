// Copyright 2023 The LUCI Authors.
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

package packedintset

import (
	"math/rand"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPack(t *testing.T) {
	t.Parallel()

	ftt.Run(`Simple test for pack.`, t, func(t *ftt.Test) {
		t.Run(`1m 1`, func(t *ftt.Test) {
			var array []int64
			for i := range 1000000 {
				array = append(array, int64(i))
			}

			data, err := Pack(array)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, 1500, should.BeGreaterThanOrEqual(len(data)))

			unpackedArray, err := Unpack(data)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, array, should.Match(unpackedArray))
		})

		t.Run(`1m 1000`, func(t *ftt.Test) {
			var array []int64
			for i := range 1000000 {
				array = append(array, int64(i*1000))
			}

			data, err := Pack(array)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(data), should.BeGreaterThan(1000))

			unpackedArray, err := Unpack(data)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, array, should.Match(unpackedArray))
		})

		t.Run(`1m pseudo`, func(t *ftt.Test) {
			var array []int64
			r := rand.New(rand.NewSource(0))
			for range 1000000 {
				array = append(array, r.Int63n(1000000))
			}
			sort.Slice(array, func(i, j int) bool {
				return array[i] <= array[j]
			})

			data, err := Pack(array)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(data), should.BeGreaterThan(2000))

			unpackedArray, err := Unpack(data)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, array, should.Match(unpackedArray))
		})

		t.Run(`empty`, func(t *ftt.Test) {
			data, err := Pack([]int64{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.BeEmpty)

			unpackedArray, err := Unpack([]byte{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, unpackedArray, should.BeEmpty)
		})
	})
}
