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

package submission

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/data/stringset"
)

// TODO(yiwzhang): clean up all the following temporary structs once we
// have the formal data model in the new CV.

type CL struct {
	Key  string
	Deps []Dep
}

type Dep struct {
	Key         string
	Requirement RequirementType
}

type RequirementType int8

const (
	Soft RequirementType = iota
	Hard
)

// OptimizeOrders computes best order for submission of CLs based on their
// dependencies. CLs are toposorted and in case of presence of dependency
// cycle, break cycle by arbitrarily but deterministically ignoring soft
// requirement dependencies and approximately minimizing the number of
// broken soft dependencies.
//
// Panics if duplicate CLs are provided or the hard requirement dependencies
// solely can form a cycle (This is not possible within current CV context as
// hard requirement deps can only be established if CLs follow Git's
// child -> parent relationship).
//
// Overall, this is as hard as a minimum feedback arc set, which is NP-Hard:
// https://en.wikipedia.org/wiki/Feedback_arc_set#Minimum_feedback_arc_set
// However, approximation is fine within CV context so long as hard
// dependencies are satisfied.
func OptimizeOrders(cls []CL) []CL {
	return (&optimizer{CLs: cls}).optimize()
}

type optimizer struct {
	CLs           []CL
	unvisitedCLs  map[string]CL
	brokenDeps    map[brokenDep]struct{}
	cycleDetector stringset.Set
	result        []CL
}

type brokenDep struct {
	src, dest string
}

// optimize is dfs-based toposort which backtracks if cycle is found and removes
// the latest followed dep that is soft requirement dep.
//
// Denoting V=len(cls), E=number of dependencies edges (bounded by O(V^2)),
// the runtime is bounded by O(V*E), which worst case can be O(V**3).
// In CV context, E is usually O(V), so this is fine.
//
// This can be reduced to O(max(E, V)) by first computing isomorphic
// graph of connected components via hard requirement deps and then
// running toposorts on a graph of components AND inside each component
// separately.
// Alternative idea for improvement is to could accumulate a skiplist style
// datastructure during iteration too, i.e. from node X what is the next
// guaranteed-reachable node Y. Then during backtracking, just skip forward
// over chains of hard requirement changes. Also, with a reciprocal structure,
// during 'skip back' phase one can skip directly to the nearest soft dep.
func (t *optimizer) optimize() []CL {
	cls := make([]CL, len(t.CLs))
	copy(cls, t.CLs)
	sort.SliceStable(cls, func(i, j int) bool {
		return cls[i].Key < cls[j].Key
	})
	t.unvisitedCLs = map[string]CL{}
	for _, cl := range cls {
		if _, ok := t.unvisitedCLs[cl.Key]; ok {
			panic(fmt.Sprintf("duplicate cl: %s", cl.Key))
		}
		t.unvisitedCLs[cl.Key] = cl
	}

	t.brokenDeps = map[brokenDep]struct{}{}
	t.cycleDetector = stringset.Set{}
	t.result = make([]CL, 0, len(cls))

	for _, cl := range cls {
		if t.visit(cl) {
			// A cycle was formed via hard requirement deps, which should not
			// happen.
			panic(fmt.Sprintf("cycle detected for cl: %s", cl.Key))
		}
	}
	return t.result
}

func (t *optimizer) visit(cl CL) bool {
	if _, ok := t.unvisitedCLs[cl.Key]; !ok {
		return false
	}
	if t.cycleDetector.Has(cl.Key) {
		return true
	}
	t.cycleDetector.Add(cl.Key)

	for _, dep := range t.remainingDeps(cl) {
		for t.visit(t.unvisitedCLs[dep.Key]) {
			// Going deeper forms a cycle which can't be broken up at deeper levels.
			if dep.Requirement == Hard {
				// Can't break it at this level either, backtrack.
				t.cycleDetector.Del(cl.Key)
				return true
			}
			// Greedily break cycle by removing this dep. This is why algo is
			// only an approximation.
			t.brokenDeps[brokenDep{cl.Key, dep.Key}] = struct{}{}
			break
		}
	}
	t.cycleDetector.Del(cl.Key)
	delete(t.unvisitedCLs, cl.Key)
	t.result = append(t.result, cl)
	return false
}

func (t *optimizer) remainingDeps(cl CL) []Dep {
	ret := make([]Dep, 0, len(cl.Deps))
	for _, dep := range cl.Deps {
		if _, ok := t.unvisitedCLs[dep.Key]; ok {
			if _, ok := t.brokenDeps[brokenDep{cl.Key, dep.Key}]; !ok {
				ret = append(ret, dep)
			}
		}
	}
	return ret
}
