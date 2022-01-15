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
	"testing"

	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRealms(t *testing.T) {
	t.Parallel()

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
