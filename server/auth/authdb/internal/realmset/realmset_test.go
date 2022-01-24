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

package realmset

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRealms(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	grp := groups(map[string][]string{
		"g1": {},
		"g2": {},
	})

	Convey("Works", t, func() {
		r, err := Build(&protocol.Realms{
			ApiVersion: ExpectedAPIVersion,
			Permissions: []*protocol.Permission{
				{Name: "luci.dev.testing0"},
				{Name: "luci.dev.testing1"},
				{Name: "luci.dev.testing2"},
			},
			Realms: []*protocol.Realm{
				{
					Name: "proj:r1",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
							Principals: []string{
								"group:g1",
								"group:unknown",
								"user:u1@example.com",
							},
						},
						{
							Permissions: []uint32{0, 1, 2},
							Principals: []string{
								"group:g1",
								"user:u2@example.com",
							},
						},
						{
							Permissions: []uint32{2},
							Principals:  []string{"group:g2", "user:u2@example.com"},
						},
					},
					Data: &protocol.RealmData{
						EnforceInService: []string{"a"},
					},
				},
				{
					Name: "proj:empty",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
						},
						{
							Permissions: []uint32{0, 1, 2},
						},
					},
				},
			},
		}, grp)
		So(err, ShouldBeNil)

		So(r.perms, ShouldResemble, map[string]PermissionIndex{
			"luci.dev.testing0": 0,
			"luci.dev.testing1": 1,
			"luci.dev.testing2": 2,
		})
		So(r.names.ToSortedSlice(), ShouldResemble, []string{"proj:empty", "proj:r1"})

		idx, ok := r.PermissionIndex(realms.RegisterPermission("luci.dev.testing2"))
		So(ok, ShouldBeTrue)
		So(idx, ShouldEqual, 2)

		_, ok = r.PermissionIndex(realms.RegisterPermission("luci.dev.unknown"))
		So(ok, ShouldBeFalse)

		So(r.HasRealm("proj:r1"), ShouldBeTrue)
		So(r.HasRealm("proj:empty"), ShouldBeTrue)
		So(r.HasRealm("proj:unknown"), ShouldBeFalse)

		So(r.Data("proj:r1").EnforceInService, ShouldResemble, []string{"a"})
		So(r.Data("proj:empty"), ShouldBeNil)
		So(r.Data("proj:unknown"), ShouldBeNil)

		bs := r.Bindings("proj:r1", 0)
		So(bs, ShouldHaveLength, 1)
		So(bs[0].Groups, ShouldResemble, indexes(grp, "g1"))
		So(bs[0].Idents.ToSortedSlice(), ShouldResemble, []string{"user:u1@example.com", "user:u2@example.com"})

		bs = r.Bindings("proj:r1", 1)
		So(bs, ShouldHaveLength, 1)
		So(bs[0].Groups, ShouldResemble, indexes(grp, "g1"))
		So(bs[0].Idents.ToSortedSlice(), ShouldResemble, []string{"user:u2@example.com"})

		bs = r.Bindings("proj:r1", 2)
		So(bs, ShouldHaveLength, 1)
		So(bs[0].Groups, ShouldResemble, indexes(grp, "g1", "g2"))
		So(bs[0].Idents.ToSortedSlice(), ShouldResemble, []string{"user:u2@example.com"})

		So(r.Bindings("proj:empty", 0), ShouldBeEmpty)
		So(r.Bindings("proj:unknown", 0), ShouldBeEmpty)
	})

	Convey("Conditional bindings", t, func() {
		r, err := Build(&protocol.Realms{
			ApiVersion: ExpectedAPIVersion,
			Permissions: []*protocol.Permission{
				{Name: "luci.dev.testing0"},
				{Name: "luci.dev.testing1"},
			},
			Conditions: []*protocol.Condition{
				restrict("a", "ok"),
				restrict("b", "ok"),
			},
			Realms: []*protocol.Realm{
				{
					Name: "proj:r1",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
							Principals: []string{
								"user:0@example.com",
							},
						},
						{
							Permissions: []uint32{1},
							Principals: []string{
								"user:1@example.com",
							},
						},
						{
							Permissions: []uint32{0},
							Conditions:  []uint32{0},
							Principals: []string{
								"user:0-if-0@example.com",
							},
						},
						{
							Permissions: []uint32{0},
							Conditions:  []uint32{1},
							Principals: []string{
								"user:0-if-1@example.com",
							},
						},
						{
							Permissions: []uint32{0, 1},
							Principals: []string{
								"user:01@example.com",
							},
						},
						{
							Permissions: []uint32{0, 1},
							Conditions:  []uint32{0},
							Principals: []string{
								"user:01-if-0@example.com",
							},
						},
						{
							Permissions: []uint32{0, 1},
							Conditions:  []uint32{1},
							Principals: []string{
								"user:01-if-1@example.com",
							},
						},
						{
							Permissions: []uint32{0},
							Conditions:  []uint32{0, 1},
							Principals: []string{
								"user:0-if-0&1@example.com",
							},
						},
						{
							Permissions: []uint32{1},
							Conditions:  []uint32{0, 1},
							Principals: []string{
								"user:1-if-0&1@example.com",
							},
						},
						{
							Permissions: []uint32{1},
							Conditions:  []uint32{1},
							Principals: []string{
								"user:1-if-1@example.com",
							},
						},
					},
				},
			},
		}, grp)
		So(err, ShouldBeNil)

		type pretty struct {
			cond  int // index of the condition+1 or 0 if unconditional
			users []string
		}

		prettify := func(bs []Binding) []pretty {
			out := make([]pretty, len(bs))
			for i, b := range bs {
				cond := 0
				if b.Condition != nil {
					cond = b.Condition.Index() + 1
				}
				out[i] = pretty{
					cond:  cond,
					users: b.Idents.ToSortedSlice(),
				}
			}
			return out
		}

		bs0 := r.Bindings("proj:r1", 0)
		So(prettify(bs0), ShouldResemble, []pretty{
			{cond: 0, users: []string{"user:01@example.com", "user:0@example.com"}},
			{cond: 1, users: []string{"user:0-if-0@example.com", "user:01-if-0@example.com"}},
			{cond: 2, users: []string{"user:0-if-1@example.com", "user:01-if-1@example.com"}},
			{cond: 3, users: []string{"user:0-if-0&1@example.com"}},
		})

		bs1 := r.Bindings("proj:r1", 1)
		So(prettify(bs1), ShouldResemble, []pretty{
			{cond: 0, users: []string{"user:01@example.com", "user:1@example.com"}},
			{cond: 1, users: []string{"user:01-if-0@example.com"}},
			{cond: 2, users: []string{"user:01-if-1@example.com", "user:1-if-1@example.com"}},
			{cond: 3, users: []string{"user:1-if-0&1@example.com"}},
		})

		// Now actually confirm mapping of `cond` indexes above to elementary
		// conditions from Realms proto.

		// 1 is elementary 0: attr.a==ok.
		cond1 := bs0[1].Condition
		So(cond1.Index(), ShouldEqual, 0)
		So(cond1.Eval(ctx, realms.Attrs{"a": "ok"}), ShouldBeTrue)
		So(cond1.Eval(ctx, realms.Attrs{"a": "??"}), ShouldBeFalse)

		// 2 is elementary 1: attr.b==ok.
		cond2 := bs0[2].Condition
		So(cond2.Index(), ShouldEqual, 1)
		So(cond2.Eval(ctx, realms.Attrs{"b": "ok"}), ShouldBeTrue)
		So(cond2.Eval(ctx, realms.Attrs{"b": "??"}), ShouldBeFalse)

		// 3 is elementary 0&1: attr.a==ok && attr.b==ok.
		cond3 := bs0[3].Condition
		So(cond3.Index(), ShouldEqual, 2)
		So(cond3.Eval(ctx, realms.Attrs{"a": "ok", "b": "ok"}), ShouldBeTrue)
		So(cond3.Eval(ctx, realms.Attrs{"a": "??", "b": "ok"}), ShouldBeFalse)
		So(cond3.Eval(ctx, realms.Attrs{"a": "ok", "b": "??"}), ShouldBeFalse)
	})
}

func groups(gr map[string][]string) *graph.QueryableGraph {
	g := make([]*protocol.AuthGroup, 0, len(gr))
	for name, members := range gr {
		g = append(g, &protocol.AuthGroup{
			Name:    name,
			Members: members,
		})
	}
	q, err := graph.BuildQueryable(g)
	if err != nil {
		panic(err)
	}
	return q
}

func indexes(q *graph.QueryableGraph, groups ...string) graph.SortedNodeSet {
	ns := graph.NodeSet{}
	for _, g := range groups {
		idx, ok := q.GroupIndex(g)
		if !ok {
			panic("unknown group " + g)
		}
		ns.Add(idx)
	}
	return ns.Sort()
}

func restrict(attr, val string) *protocol.Condition {
	return &protocol.Condition{
		Op: &protocol.Condition_Restrict{
			Restrict: &protocol.Condition_AttributeRestriction{
				Attribute: attr,
				Values:    []string{val},
			},
		},
	}
}
