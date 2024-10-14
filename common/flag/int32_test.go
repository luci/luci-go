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

func TestInt32Flag(t *testing.T) {
	t.Parallel()

	ftt.Run("error", t, func(t *ftt.Test) {
		var flag int32Flag
		assert.Loosely(t, flag.Set("0x01"), should.ErrLike("values must be 32-bit integers"))
		assert.Loosely(t, flag, should.BeZero)
		assert.Loosely(t, flag.Get(), should.BeZero)
		assert.Loosely(t, flag.String(), should.Equal("0"))
	})

	ftt.Run("int64", t, func(t *ftt.Test) {
		var flag int32Flag
		assert.Loosely(t, flag.Set("2147483648"), should.ErrLike("values must be 32-bit integers"))
		assert.Loosely(t, flag, should.BeZero)
		assert.Loosely(t, flag.Get(), should.BeZero)
		assert.Loosely(t, flag.String(), should.Equal("0"))
	})

	ftt.Run("zero", t, func(t *ftt.Test) {
		var flag int32Flag
		assert.Loosely(t, flag.Set("0"), should.BeNil)
		assert.Loosely(t, flag, should.BeZero)
		assert.Loosely(t, flag.Get(), should.BeZero)
		assert.Loosely(t, flag.String(), should.Equal("0"))
	})

	ftt.Run("min", t, func(t *ftt.Test) {
		var flag int32Flag
		assert.Loosely(t, flag.Set("-2147483648"), should.BeNil)
		assert.Loosely(t, flag, should.Equal(-2147483648))
		assert.Loosely(t, flag.Get(), should.Equal(-2147483648))
		assert.Loosely(t, flag.String(), should.Equal("-2147483648"))
	})

	ftt.Run("max", t, func(t *ftt.Test) {
		var flag int32Flag
		assert.Loosely(t, flag.Set("2147483647"), should.BeNil)
		assert.Loosely(t, flag, should.Equal(2147483647))
		assert.Loosely(t, flag.Get(), should.Equal(2147483647))
		assert.Loosely(t, flag.String(), should.Equal("2147483647"))
	})
}

func TestInt32(t *testing.T) {
	t.Parallel()

	ftt.Run("error", t, func(t *ftt.Test) {
		var i int32
		assert.Loosely(t, Int32(&i).Set("0x1"), should.ErrLike("values must be 32-bit integers"))
		assert.Loosely(t, i, should.BeZero)
	})

	ftt.Run("int64", t, func(t *ftt.Test) {
		var i int32
		assert.Loosely(t, Int32(&i).Set("2147483648"), should.ErrLike("values must be 32-bit integers"))
		assert.Loosely(t, i, should.BeZero)
	})

	ftt.Run("zero", t, func(t *ftt.Test) {
		var i int32
		assert.Loosely(t, Int32(&i).Set("0"), should.BeNil)
		assert.Loosely(t, i, should.BeZero)
	})

	ftt.Run("min", t, func(t *ftt.Test) {
		var i int32
		assert.Loosely(t, Int32(&i).Set("-2147483648"), should.BeNil)
		assert.Loosely(t, i, should.Equal(-2147483648))
	})

	ftt.Run("max", t, func(t *ftt.Test) {
		var i int32
		assert.Loosely(t, Int32(&i).Set("2147483647"), should.BeNil)
		assert.Loosely(t, i, should.Equal(2147483647))
	})
}
