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

package flag

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestInt64SliceFlag(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var flag int64SliceFlag
		assert.Loosely(t, flag.Set("1"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Resemble([]int64{1}))
		assert.Loosely(t, flag.String(), should.Equal("1"))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var flag int64SliceFlag
		assert.Loosely(t, flag.Set("-1"), should.BeNil)
		assert.Loosely(t, flag.Set("0"), should.BeNil)
		assert.Loosely(t, flag.Set("1"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Resemble([]int64{-1, 0, 1}))
		assert.Loosely(t, flag.String(), should.Equal("-1, 0, 1"))
	})

	ftt.Run("error", t, func(t *ftt.Test) {
		var flag int64SliceFlag
		assert.Loosely(t, flag.Set("0x00"), should.ErrLike("values must be 64-bit integers"))
		assert.Loosely(t, flag, should.BeEmpty)
		assert.Loosely(t, flag, should.BeEmpty)
		assert.Loosely(t, flag.Get(), should.BeEmpty)
		assert.Loosely(t, flag.String(), should.BeEmpty)
	})

	ftt.Run("mixed", t, func(t *ftt.Test) {
		var flag int64SliceFlag
		assert.Loosely(t, flag.Set("-1"), should.BeNil)
		assert.Loosely(t, flag.Set("0x00"), should.ErrLike("values must be 64-bit integers"))
		assert.Loosely(t, flag.Set("1"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Resemble([]int64{-1, 1}))
		assert.Loosely(t, flag.String(), should.Equal("-1, 1"))
	})
}

func TestInt64Slice(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var i []int64
		assert.Loosely(t, Int64Slice(&i).Set("1"), should.BeNil)
		assert.Loosely(t, i, should.Resemble([]int64{1}))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var i []int64
		assert.Loosely(t, Int64Slice(&i).Set("-1"), should.BeNil)
		assert.Loosely(t, Int64Slice(&i).Set("0"), should.BeNil)
		assert.Loosely(t, Int64Slice(&i).Set("1"), should.BeNil)
		assert.Loosely(t, i, should.Resemble([]int64{-1, 0, 1}))
	})

	ftt.Run("error", t, func(t *ftt.Test) {
		var i []int64
		assert.Loosely(t, Int64Slice(&i).Set("0x00"), should.ErrLike("values must be 64-bit integers"))
		assert.Loosely(t, i, should.BeEmpty)
	})

	ftt.Run("mixed", t, func(t *ftt.Test) {
		var i []int64
		assert.Loosely(t, Int64Slice(&i).Set("-1"), should.BeNil)
		assert.Loosely(t, Int64Slice(&i).Set("0x00"), should.ErrLike("values must be 64-bit integers"))
		assert.Loosely(t, Int64Slice(&i).Set("1"), should.BeNil)
		assert.Loosely(t, i, should.Resemble([]int64{-1, 1}))
	})
}
