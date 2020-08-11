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

// TODO(yiwzhang): move this functionality to its own package or a package
// that has much narrower scope.

package submission

import (
	"sort"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// TODO(yiwzhang): clean up all the following structs once we have formal data
// model for new CV.

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
func ComputeOrder(cls []CL) ([]CL, error) {
	ret, _, err := compute(cls)
	return ret, err
}

type brokenDep struct {
	src, dest string
}

// optimize is DFS-based toposort which backtracks if a cycle is found and
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
func compute(cls []CL) ([]CL, int, error) {
	ret := make([]CL, 0, len(cls))
	remainingCLs := make(map[string]CL, len(cls))
	for _, cl := range cls {
		if _, ok := remainingCLs[cl.Key]; ok {
			return nil, 0, errors.Reason("duplicate cl: %s", cl.Key).Err()
		}
		remainingCLs[cl.Key] = cl
	}
	brokenDeps := map[brokenDep]struct{}{}
	cycleDetector := stringset.Set{}

	// visit returns whether a cycle was detected that can't be resolved yet.
	var visit func(cl CL) bool
	visit = func(cl CL) bool {
		if _, ok := remainingCLs[cl.Key]; !ok {
			return false
		}
		if cycleDetector.Has(cl.Key) {
			return true
		}
		cycleDetector.Add(cl.Key)

		for _, dep := range cl.Deps {
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
			if _, ok := remainingCLs[dep.Key]; !ok {
				continue
			}
			if _, ok := brokenDeps[brokenDep{cl.Key, dep.Key}]; ok {
				continue
			}
			for visit(remainingCLs[dep.Key]) {
				// Going deeper forms a cycle which can't be broken up at deeper levels.
				if dep.Requirement == Hard {
					// Can't break it at this level either, backtrack.
					cycleDetector.Del(cl.Key)
					return true
				}
				// Greedily break cycle by removing this dep. This is why this
				// algorithm is only an approximation.
				brokenDeps[brokenDep{cl.Key, dep.Key}] = struct{}{}
				break
			}
		}
		cycleDetector.Del(cl.Key)
		delete(remainingCLs, cl.Key)
		ret = append(ret, cl)
		return false
	}

	sortedCLs := make([]CL, len(cls))
	copy(sortedCLs, cls)
	sort.SliceStable(sortedCLs, func(i, j int) bool {
		return sortedCLs[i].Key < sortedCLs[j].Key
	})
	for _, cl := range sortedCLs {
		if visit(cl) {
			// A cycle was formed via hard requirement deps, which should not
			// happen.
			return nil, 0, errors.Reason("cycle detected for cl: %s", cl.Key).Err()
		}
	}
	return ret, len(brokenDeps), nil
}
