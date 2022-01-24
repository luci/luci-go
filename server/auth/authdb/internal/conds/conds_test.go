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

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("AttributeRestriction empty", t, func() {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", nil),
		})

		cond, err := builder.Condition([]uint32{0})
		So(err, ShouldBeNil)
		So(cond, ShouldNotBeNil)

		So(cond.Eval(ctx, realms.Attrs{"a": "b"}), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"a": ""}), ShouldBeFalse)
	})

	Convey("AttributeRestriction non-empty", t, func() {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
		})

		cond, err := builder.Condition([]uint32{0})
		So(err, ShouldBeNil)
		So(cond, ShouldNotBeNil)

		So(cond.Eval(ctx, nil), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"a": "val1"}), ShouldBeTrue)
		So(cond.Eval(ctx, realms.Attrs{"a": "val2"}), ShouldBeTrue)
		So(cond.Eval(ctx, realms.Attrs{"a": "val3"}), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"b": "val1"}), ShouldBeFalse)
	})

	Convey("Unrecognized elementary condition", t, func() {
		builder := NewBuilder([]*protocol.Condition{
			{},
		})

		cond, err := builder.Condition([]uint32{0})
		So(err, ShouldBeNil)
		So(cond, ShouldNotBeNil)

		So(cond.Eval(ctx, nil), ShouldBeFalse)
	})

	Convey("Empty Condition", t, func() {
		builder := NewBuilder(nil)

		cond, err := builder.Condition(nil)
		So(err, ShouldBeNil)
		So(cond, ShouldBeNil)
	})

	Convey("Condition ANDs elementary conditions", t, func() {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
			restrict("b", []string{"val1", "val2"}),
		})

		cond, err := builder.Condition([]uint32{0, 1})
		So(err, ShouldBeNil)
		So(cond, ShouldNotBeNil)

		So(cond.Eval(ctx, nil), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"a": "val1"}), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"b": "val1"}), ShouldBeFalse)
		So(cond.Eval(ctx, realms.Attrs{"a": "val1", "b": "val1"}), ShouldBeTrue)
		So(cond.Eval(ctx, realms.Attrs{"a": "val2", "b": "val2"}), ShouldBeTrue)
		So(cond.Eval(ctx, realms.Attrs{"a": "xxxx", "b": "val1"}), ShouldBeFalse)
	})

	Convey("Conditions are cached", t, func() {
		builder := NewBuilder([]*protocol.Condition{
			restrict("a", []string{"val1", "val2"}),
			restrict("b", []string{"val1", "val2"}),
			restrict("c", []string{"val1", "val2"}),
		})

		cond1, err := builder.Condition([]uint32{0, 1})
		So(err, ShouldBeNil)
		So(cond1, ShouldNotBeNil)
		So(cond1.Index(), ShouldEqual, 0)

		cond2, err := builder.Condition([]uint32{0, 1})
		So(err, ShouldBeNil)
		So(cond2, ShouldEqual, cond1) // the exact same pointer

		cond3, err := builder.Condition([]uint32{1, 0})
		So(err, ShouldBeNil)
		So(cond3, ShouldNotEqual, cond1) // different, the order matters
		So(cond3.Index(), ShouldEqual, 1)

		cond4, err := builder.Condition([]uint32{0})
		So(err, ShouldBeNil)
		So(cond4, ShouldNotEqual, cond1)
		So(cond4.Index(), ShouldEqual, 2)

		cond5, err := builder.Condition([]uint32{0, 2})
		So(err, ShouldBeNil)
		So(cond5, ShouldNotEqual, cond1)
		So(cond5.Index(), ShouldEqual, 3)
	})

	Convey("Out of bounds", t, func() {
		builder := NewBuilder(nil)

		cond, err := builder.Condition([]uint32{0})
		So(err, ShouldErrLike, "condition index is out of bounds: 0 >= 0")
		So(cond, ShouldBeNil)
	})
}
