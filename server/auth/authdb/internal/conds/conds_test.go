// Copyright 2022 The LUCI Authors.
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

package conds

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestConds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	restrict := func(attr string, vals []string) *protocol.Condition {
		return &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: attr,
					Values:    vals,
				},
			},
		}
	}

	ftt.Run("AttributeRestriction empty", t, func(t *ftt.Test) {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", nil),
		})

		cond, err := builder.Condition([]uint32{0})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond, should.NotBeNil)

		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "b"}), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": ""}), should.BeFalse)
	})

	ftt.Run("AttributeRestriction non-empty", t, func(t *ftt.Test) {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
		})

		cond, err := builder.Condition([]uint32{0})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond, should.NotBeNil)

		assert.Loosely(t, cond.Eval(ctx, nil), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val1"}), should.BeTrue)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val2"}), should.BeTrue)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val3"}), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"b": "val1"}), should.BeFalse)
	})

	ftt.Run("Unrecognized elementary condition", t, func(t *ftt.Test) {
		builder := NewBuilder([]*protocol.Condition{
			{},
		})

		cond, err := builder.Condition([]uint32{0})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond, should.NotBeNil)

		assert.Loosely(t, cond.Eval(ctx, nil), should.BeFalse)
	})

	ftt.Run("Empty Condition", t, func(t *ftt.Test) {
		builder := NewBuilder(nil)

		cond, err := builder.Condition(nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond, should.BeNil)
	})

	ftt.Run("Condition ANDs elementary conditions", t, func(t *ftt.Test) {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
			restrict("b", []string{"val1", "val2"}),
		})

		cond, err := builder.Condition([]uint32{0, 1})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond, should.NotBeNil)

		assert.Loosely(t, cond.Eval(ctx, nil), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val1"}), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"b": "val1"}), should.BeFalse)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val1", "b": "val1"}), should.BeTrue)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "val2", "b": "val2"}), should.BeTrue)
		assert.Loosely(t, cond.Eval(ctx, realms.Attrs{"a": "xxxx", "b": "val1"}), should.BeFalse)
	})

	ftt.Run("Conditions are cached", t, func(t *ftt.Test) {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
			restrict("b", []string{"val1", "val2"}),
			restrict("c", []string{"val1", "val2"}),
		})

		cond1, err := builder.Condition([]uint32{0, 1})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond1, should.NotBeNil)
		assert.Loosely(t, cond1.Index(), should.BeZero)

		cond2, err := builder.Condition([]uint32{0, 1})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond2, should.Equal(cond1)) // the exact same pointer

		cond3, err := builder.Condition([]uint32{1, 0})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond3, should.NotEqual(cond1)) // different, the order matters
		assert.Loosely(t, cond3.Index(), should.Equal(1))

		cond4, err := builder.Condition([]uint32{0})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond4, should.NotEqual(cond1))
		assert.Loosely(t, cond4.Index(), should.Equal(2))

		cond5, err := builder.Condition([]uint32{0, 2})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cond5, should.NotEqual(cond1))
		assert.Loosely(t, cond5.Index(), should.Equal(3))
	})

	ftt.Run("Out of bounds", t, func(t *ftt.Test) {
		builder := NewBuilder(nil)

		cond, err := builder.Condition([]uint32{0})
		assert.Loosely(t, err, should.ErrLike("condition index is out of bounds: 0 >= 0"))
		assert.Loosely(t, cond, should.BeNil)
	})
}
