// Copyright 2020 The LUCI Authors.
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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestSequence(t *testing.T) {
	t.Parallel()

	ftt.Run("GenerateSequenceNumbers", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("not found", func(t *ftt.Test) {
			t.Run("zero", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(1))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(1))
			})

			t.Run("one", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(1))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(2))
			})

			t.Run("many", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(1))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(11))
			})
		})

		t.Run("found", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &NumberSequence{
				ID:   "seq",
				Next: 2,
			}), should.BeNil)

			t.Run("zero", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(2))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(2))
			})

			t.Run("one", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(2))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(3))
			})

			t.Run("many", func(t *ftt.Test) {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(2))

				seq, err = GenerateSequenceNumbers(ctx, "seq", 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, seq, should.Equal(12))
			})
		})
	})
}
