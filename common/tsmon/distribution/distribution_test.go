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

package distribution

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNew(t *testing.T) {
	ftt.Run("Passing nil uses the default bucketer", t, func(t *ftt.Test) {
		d := New(nil)
		assert.Loosely(t, d.Bucketer(), should.Equal(DefaultBucketer))
	})
}

func TestAdd(t *testing.T) {
	ftt.Run("Add", t, func(t *ftt.Test) {
		d := New(FixedWidthBucketer(10, 2))
		assert.Loosely(t, d.Sum(), should.BeZero)
		assert.Loosely(t, d.Count(), should.BeZero)

		d.Add(1)
		assert.Loosely(t, d.Buckets(), should.Resemble([]int64{0, 1}))
		d.Add(10)
		assert.Loosely(t, d.Buckets(), should.Resemble([]int64{0, 1, 1}))
		d.Add(20)
		assert.Loosely(t, d.Buckets(), should.Resemble([]int64{0, 1, 1, 1}))
		d.Add(30)
		assert.Loosely(t, d.Buckets(), should.Resemble([]int64{0, 1, 1, 2}))
		assert.Loosely(t, d.Sum(), should.Equal(61.0))
		assert.Loosely(t, d.Count(), should.Equal(4))
	})
}

func TestClone(t *testing.T) {
	ftt.Run("Clone empty", t, func(t *ftt.Test) {
		d := New(FixedWidthBucketer(10, 2))
		assert.Loosely(t, d, should.Resemble(d.Clone()))
	})

	ftt.Run("Clone populated", t, func(t *ftt.Test) {
		d := New(FixedWidthBucketer(10, 2))
		d.Add(1)
		d.Add(10)
		d.Add(20)

		clone := d.Clone()
		assert.Loosely(t, d, should.Resemble(clone))

		d.Add(30)
		assert.Loosely(t, d, should.NotResemble(clone))
	})
}
