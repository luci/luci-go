// Copyright 2023 The LUCI Authors.
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

package realmsinternals

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestConditionsSet(t *testing.T) {
	t.Parallel()
	restriction := func(attr string, values []string) *realmsconf.Condition {
		return &realmsconf.Condition{
			Op: &realmsconf.Condition_Restrict{
				Restrict: &realmsconf.Condition_AttributeRestriction{
					Attribute: attr,
					Values:    values,
				},
			},
		}
	}

	Convey("test key", t, func() {
		cond1 := &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: "attr",
					Values:    []string{"test"},
				},
			},
		}
		cond2 := &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: "attr",
					Values:    []string{"test"},
				},
			},
		}
		// same contents == same key
		cond1Key, cond2Key := conditionKey(cond1), conditionKey(cond2)
		So(cond1Key, ShouldEqual, cond2Key)

		condEmpty := &protocol.Condition{}
		So(conditionKey(condEmpty), ShouldEqual, "")
	})

	Convey("errors", t, func() {
		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
		}

		r1 := restriction("a", []string{"1", "2"})
		r2 := restriction("b", []string{"1"})

		So(cs.addCond(r1), ShouldBeNil)
		cs.finalize()
		So(cs.addCond(r2), ShouldEqual, ErrFinalized)
	})

	Convey("works", t, func() {

		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
			finalized:    false,
		}

		r1 := restriction("b", []string{"1", "2"})
		r2 := restriction("a", []string{"2", "1", "1"})
		r3 := restriction("a", []string{"1", "2"})
		r4 := restriction("a", []string{"3", "4"})

		So(cs.addCond(r1), ShouldBeNil)
		So(cs.addCond(r1), ShouldBeNil)
		So(cs.addCond(r2), ShouldBeNil)
		So(cs.addCond(r3), ShouldBeNil)
		So(cs.addCond(r4), ShouldBeNil)
		out := cs.finalize()
		expected := []*protocol.Condition{
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "a",
						Values:    []string{"1", "2"},
					},
				},
			},
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "a",
						Values:    []string{"3", "4"},
					},
				},
			},
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "b",
						Values:    []string{"1", "2"},
					},
				},
			},
		}
		So(out, ShouldResembleProto, expected)

		So(cs.indexes([]*realmsconf.Condition{r1}), ShouldResemble, []uint32{2})
		So(cs.indexes([]*realmsconf.Condition{r2}), ShouldResemble, []uint32{0})
		So(cs.indexes([]*realmsconf.Condition{r3}), ShouldResemble, []uint32{0})
		So(cs.indexes([]*realmsconf.Condition{r4}), ShouldResemble, []uint32{1})
		inds := cs.indexes([]*realmsconf.Condition{r1, r2, r3, r4})
		So(inds, ShouldResemble, []uint32{0, 1, 2})
	})
}
