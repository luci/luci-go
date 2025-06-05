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

package submit

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// ComputeOrder computes the best order for submission of CLs based on
// their dependencies. Dependency cycle is broken by arbitrarily but
// deterministically breaking soft requirement dependencies and approximately
// minimizing their number.
//
// Returns error if duplicate CLs are provided or the hard requirement
// dependencies solely form a cycle (This should not be possible within current
// CV context as hard requirement deps can only be established if CLs follow
// Git's child -> parent relationship).
//
// Overall, this is as hard as a minimum feedback arc set, which is NP-Hard:
// https://en.wikipedia.org/wiki/Feedback_arc_set#Minimum_feedback_arc_set
// However, approximation is fine within CV context so long as hard
// dependencies are satisfied.
func ComputeOrder(cls []*run.RunCL) ([]*run.RunCL, error) {
	ret, _, err := compute(cls)
	return ret, err
}

type brokenDep struct {
	src, dest common.CLID
}

// compute is DFS-based toposort which backtracks if a cycle is found and
// removes the last-followed soft requirement dependency.
//
// Denoting V=len(cls), E=number of dependency edges (bounded by O(V^2)),
// the runtime is bounded by O(V*E), which worst case can be O(V**3).
//
// This can be reduced to O(max(E, V)) by first computing isomorphic
// graph of connected components via hard requirement deps and then
// running toposorts on a graph of components AND inside each component
// separately.
//
// Alternative idea for improvement is to could accumulate a skiplist style
// data structure during iteration too, i.e. from node X what is the next
// guaranteed-reachable node Y. Then during backtracking, just skip forward
// over chains of hard requirement changes. Also, with a reciprocal structure,
// during 'skip back' phase one can skip directly to the nearest soft dep.
func compute(cls []*run.RunCL) ([]*run.RunCL, int, error) {
	ret := make([]*run.RunCL, 0, len(cls))
	remainingCLs := make(map[common.CLID]*run.RunCL, len(cls))
	for _, cl := range cls {
		if _, ok := remainingCLs[cl.ID]; ok {
			return nil, 0, errors.Fmt("duplicate cl: %d", cl.ID)
		}
		remainingCLs[cl.ID] = cl
	}
	brokenDeps := map[brokenDep]struct{}{}
	cycleDetector := common.CLIDsSet{}

	// visit returns whether a cycle was detected that can't be resolved yet.
	var visit func(cl *run.RunCL) bool
	visit = func(cl *run.RunCL) bool {
		if _, ok := remainingCLs[cl.ID]; !ok {
			return false
		}
		if cycleDetector.Has(cl.ID) {
			return true
		}
		cycleDetector.Add(cl.ID)

		for _, dep := range cl.Detail.GetDeps() {
			// TODO(yiwzhang): Strictly speaking, the time complexity for hashtable
			// access is not strictly O(1) considering the time spent on hash func
			// and comparing the output value (Denoted it as HASH(key)). Therefore,
			// the time we spent on the following lines is O(E*V*HASH) which is
			// O(V^3*HASH) in worst case. This can be improved by pre-computing
			// Deps of each CLs to replace its key with the index of the CL
			// (requires O(E*HASH)). Then, we can make remainingCLs a bitmask so
			// that the time spent on the following line will reduce to O(E*V*1).
			// We are gonna defer this improvement till the struct for `Dep` is
			// clear to us.
			depCLID := common.CLID(dep.GetClid())
			if _, ok := remainingCLs[depCLID]; !ok {
				continue
			}
			if _, ok := brokenDeps[brokenDep{cl.ID, depCLID}]; ok {
				continue
			}
			for visit(remainingCLs[depCLID]) {
				// Going deeper forms a cycle which can't be broken up at deeper levels.
				switch dep.GetKind() {
				case changelist.DepKind_SOFT:
				case changelist.DepKind_HARD:
					// Can't break it at this level either, backtrack.
					cycleDetector.Del(cl.ID)
					return true
				default:
					panic(fmt.Errorf("unknown dep kind %s", dep.GetKind()))
				}
				// Greedily break cycle by removing this dep. This is why this
				// algorithm is only an approximation.
				brokenDeps[brokenDep{cl.ID, depCLID}] = struct{}{}
				break
			}
		}
		cycleDetector.Del(cl.ID)
		delete(remainingCLs, cl.ID)
		ret = append(ret, cl)
		return false
	}

	sortedCLs := make([]*run.RunCL, len(cls))
	copy(sortedCLs, cls)
	sort.SliceStable(sortedCLs, func(i, j int) bool {
		return sortedCLs[i].ID < sortedCLs[j].ID
	})
	for _, cl := range sortedCLs {
		if visit(cl) {
			// A cycle was formed via hard requirement deps, which should not
			// happen.
			return nil, 0, errors.Fmt("cycle detected for cl: %d", cl.ID)
		}
	}
	return ret, len(brokenDeps), nil
}
