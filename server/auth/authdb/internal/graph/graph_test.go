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
	"sort"
	"testing"

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
