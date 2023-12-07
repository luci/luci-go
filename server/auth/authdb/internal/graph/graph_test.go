// Copyright 2019 The LUCI Authors.
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

package graph

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/authdb/internal/globset"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func mkGraph(adj map[string][]string) *Graph {
	groups := make([]*protocol.AuthGroup, 0, len(adj))
	for name, nested := range adj {
		groups = append(groups, &protocol.AuthGroup{
			Name:   name,
			Nested: nested,
		})
	}
	g, err := Build(groups)
	if err != nil {
		panic(err)
	}
	return g
}

func names(g *Graph, ns NodeSet) []string {
	nm := make([]string, 0, len(ns))
	g.Visit(ns, func(n *Node) error {
		nm = append(nm, n.Name)
		return nil
	})
	sort.Strings(nm)
	return nm
}

func descendants(g *Graph, node string) []string {
	for _, n := range g.Nodes {
		if n.Name == node {
			return names(g, g.Descendants(n.Index))
		}
	}
	return nil
}

func ancestors(g *Graph, node string) []string {
	for _, n := range g.Nodes {
		if n.Name == node {
			return names(g, g.Ancestors(n.Index))
		}
	}
	return nil
}

func TestGraph(t *testing.T) {
	t.Parallel()

	Convey("Linear", t, func() {
		g := mkGraph(map[string][]string{
			"1": {"2"},
			"2": {"3"},
			"3": nil,
		})

		So(descendants(g, "1"), ShouldResemble, []string{"1", "2", "3"})
		So(descendants(g, "2"), ShouldResemble, []string{"2", "3"})
		So(descendants(g, "3"), ShouldResemble, []string{"3"})

		So(ancestors(g, "1"), ShouldResemble, []string{"1"})
		So(ancestors(g, "2"), ShouldResemble, []string{"1", "2"})
		So(ancestors(g, "3"), ShouldResemble, []string{"1", "2", "3"})
	})

	Convey("Tree", t, func() {
		g := mkGraph(map[string][]string{
			"root": {"l1", "r1"},
			"l1":   {"l2", "r2"},
			"r1":   nil,
			"l2":   nil,
			"r2":   nil,
		})
		So(descendants(g, "root"), ShouldResemble, []string{"l1", "l2", "r1", "r2", "root"})
		So(descendants(g, "l1"), ShouldResemble, []string{"l1", "l2", "r2"})
		So(ancestors(g, "r2"), ShouldResemble, []string{"l1", "r2", "root"})
	})

	Convey("Diamond", t, func() {
		g := mkGraph(map[string][]string{
			"root": {"l", "r"},
			"l":    {"leaf"},
			"r":    {"leaf"},
			"leaf": nil,
		})
		So(descendants(g, "root"), ShouldResemble, []string{"l", "leaf", "r", "root"})
		So(ancestors(g, "leaf"), ShouldResemble, []string{"l", "leaf", "r", "root"})
	})

	Convey("Cycle", t, func() {
		// Cycles aren't allowed in AuthDB, but we make sure if they happen for
		// whatever reason, Graph doesn't get stuck in an endless loop.
		g := mkGraph(map[string][]string{
			"1": {"2"},
			"2": {"1"},
		})
		// Note: in presence of cycles the results of calls below generally depend
		// on order they were called.
		So(descendants(g, "1"), ShouldResemble, []string{"1", "2"})
		So(descendants(g, "2"), ShouldResemble, []string{"2"})
		So(ancestors(g, "1"), ShouldResemble, []string{"1", "2"})
		So(ancestors(g, "2"), ShouldResemble, []string{"2"})
	})

	Convey("Bad nested group reference", t, func() {
		g := mkGraph(map[string][]string{"1": {"missing"}})
		So(descendants(g, "1"), ShouldResemble, []string{"1"})
	})
}

func TestNodeSet(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ns1 := make(NodeSet, 0)
		ns1.Add(5)
		ns1.Add(3)

		ns2 := make(NodeSet, 0)
		ns2.Add(10)
		ns2.Add(5)

		ns3 := make(NodeSet, 0)
		ns3.Update(ns1)
		ns3.Update(ns2)

		sorted := ns3.Sort()
		So(sorted, ShouldResemble, SortedNodeSet{3, 5, 10})

		So(sorted.Has(1), ShouldBeFalse)
		So(sorted.Has(3), ShouldBeTrue)
		So(sorted.Has(5), ShouldBeTrue)
		So(sorted.Has(10), ShouldBeTrue)
		So(sorted.Has(11), ShouldBeFalse)

		So(sorted.Intersects(SortedNodeSet{}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{1}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{1, 2}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{1, 2, 4}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{1, 2, 4, 11}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{11}), ShouldBeFalse)
		So(sorted.Intersects(SortedNodeSet{3}), ShouldBeTrue)
		So(sorted.Intersects(SortedNodeSet{1, 2, 3}), ShouldBeTrue)
		So(sorted.Intersects(SortedNodeSet{10, 11}), ShouldBeTrue)
		So(sorted.Intersects(SortedNodeSet{5}), ShouldBeTrue)

		So(sorted.MapKey(), ShouldResemble, "\x03\x00\x05\x00\x0a\x00")
	})
}

func TestQueryable(t *testing.T) {
	t.Parallel()

	Convey("Globs map", t, func() {
		q, err := BuildQueryable([]*protocol.AuthGroup{
			{
				Name:   "root",
				Globs:  []string{"user:*@1.example.com", "user:*@2.example.com"},
				Nested: []string{"child1"},
			},
			{
				Name:   "child1",
				Globs:  []string{"user:*@2.example.com"},
				Nested: []string{"child2", "no globs"},
			},
			{
				Name:  "child2",
				Globs: []string{"user:*@3.example.com"},
			},
			{
				Name: "no globs",
			},
			{
				Name:  "separate",
				Globs: []string{"user:*@3.example.com", "user:*@2.example.com"},
			},
		})
		So(err, ShouldBeNil)
		So(stringifyGlobMap(q.globs), ShouldResemble, map[NodeIndex]string{
			0: "user:^((.*@1\\.example\\.com)|(.*@2\\.example\\.com)|(.*@3\\.example\\.com))$",
			1: "user:^((.*@2\\.example\\.com)|(.*@3\\.example\\.com))$",
			2: "user:^.*@3\\.example\\.com$",
			4: "user:^((.*@2\\.example\\.com)|(.*@3\\.example\\.com))$",
		})

		// Identical GlobSet's are shared by reference.
		a := q.globs[1]["user"]
		b := q.globs[4]["user"]
		So(a == b, ShouldBeTrue)

		So(q.IsMember("user:a@3.example.com", "root"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:a@3.example.com", "child1"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:a@3.example.com", "child2"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:a@3.example.com", "no globs"), ShouldEqual, IdentIsNotMember)

		So(q.IsMember("user:a@1.example.com", "root"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:a@1.example.com", "child1"), ShouldEqual, IdentIsNotMember)
		So(q.IsMember("user:a@1.example.com", "child2"), ShouldEqual, IdentIsNotMember)
	})

	Convey("Memberships map", t, func() {
		q, err := BuildQueryable([]*protocol.AuthGroup{
			{
				Name:    "root", // 0
				Members: []string{"user:1@example.com"},
				Nested:  []string{"child1", "child2"},
			},
			{
				Name:    "child1", // 1
				Members: []string{"user:1@example.com", "user:2@example.com"},
				Nested:  []string{"child2"},
			},
			{
				Name:    "child2", // 2
				Members: []string{"user:3@example.com"},
			},
			{
				Name: "standalone", // 3
				Members: []string{
					"user:1@example.com",
					"user:2@example.com",
					"user:3@example.com",
					"user:4@example.com",
				},
			},
		})
		So(err, ShouldBeNil)

		So(q.memberships, ShouldResemble, map[identity.Identity]SortedNodeSet{
			"user:1@example.com": {0, 1, 3},
			"user:2@example.com": {0, 1, 3},
			"user:3@example.com": {0, 1, 2, 3},
			"user:4@example.com": {3},
		})

		// Identical SortedNodeSet's are shared by reference.
		a := q.memberships["user:1@example.com"]
		b := q.memberships["user:2@example.com"]
		So(&a[0] == &b[0], ShouldBeTrue)

		So(q.IsMember("user:1@example.com", "root"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:1@example.com", "child1"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:1@example.com", "child2"), ShouldEqual, IdentIsNotMember)
		So(q.IsMember("user:1@example.com", "standalone"), ShouldEqual, IdentIsMember)
		So(q.IsMember("user:1@example.com", "unknown"), ShouldEqual, GroupIsUnknown)
	})

	Convey("IsMemberOfAny", t, func() {
		q, err := BuildQueryable([]*protocol.AuthGroup{
			{
				Name:    "root",
				Members: []string{"user:1@example.com"},
				Nested:  []string{"child1", "child2"},
			},
			{
				Name:   "child1",
				Nested: []string{"child2"},
			},
			{
				Name:    "child2",
				Members: []string{"user:3@example.com"},
				Globs:   []string{"user:*glob@example.com"},
			},
			{
				Name:    "standalone",
				Members: []string{"user:z@example.com"},
			},
		})
		So(err, ShouldBeNil)

		root, _ := q.GroupIndex("root")
		standalone, _ := q.GroupIndex("standalone")

		q1 := q.MembershipsQueryCache("user:1@example.com")
		So(q1.IsMemberOfAny([]NodeIndex{root, standalone}), ShouldBeTrue)

		q2 := q.MembershipsQueryCache("user:3@example.com")
		So(q2.IsMemberOfAny([]NodeIndex{root, standalone}), ShouldBeTrue)
		So(q2.IsMemberOfAny([]NodeIndex{standalone}), ShouldBeFalse)

		q3 := q.MembershipsQueryCache("user:glob@example.com")
		So(q3.IsMemberOfAny([]NodeIndex{root, standalone}), ShouldBeTrue)
	})
}

func stringifyGlobMap(gl map[NodeIndex]globset.GlobSet) map[NodeIndex]string {
	globs := map[NodeIndex]string{}
	for idx, globSet := range gl {
		g := ""
		for k, v := range globSet {
			g += fmt.Sprintf("%s:%s", k, v)
		}
		globs[idx] = g
	}
	return globs
}

func BenchmarkSortedNodeSetHas(b *testing.B) {
	rnd := rand.New(rand.NewSource(123))

	ns := genRandomSortedNodeSet(rnd, 50, nil)
	median := ns[len(ns)/2]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns.Has(median)
	}
}

func BenchmarkSortedNodeSetIntersect(b *testing.B) {
	rnd := rand.New(rand.NewSource(123))

	ns1 := genRandomSortedNodeSet(rnd, 50, func(i NodeIndex) NodeIndex {
		return i * 2
	})
	ns2 := genRandomSortedNodeSet(rnd, 5, func(i NodeIndex) NodeIndex {
		return i*2 + 1
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns1.Intersects(ns2)
	}
}

func genRandomSortedNodeSet(rnd *rand.Rand, l int, f func(NodeIndex) NodeIndex) (ns SortedNodeSet) {
	if f == nil {
		f = func(i NodeIndex) NodeIndex { return i }
	}
	var last NodeIndex
	for i := 0; i < l; i++ {
		ns = append(ns, f(last))
		last += NodeIndex(rnd.Intn(10))
	}
	return
}
