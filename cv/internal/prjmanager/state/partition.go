// Copyright 2021 The LUCI Authors.
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

package state

import (
	"fmt"

	"go.chromium.org/luci/common/data/disjointset"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// repartition updates components.
//
// On success, guarantees the following:
//   - .Pcls contains all referenced by components CLs.
//     However, some deps of those CLs may be missing.
//   - .Pcls doesn't contain any CLs not in components.
//   - .CreatedRuns is nil,
//   - .RepartitionRequired is false.
//   - pclIndex is up-to-date.
func (s *State) repartition(cat *categorizedCLs) {
	s.ensurePCLIndex()
	plan := s.planPartition(cat)
	s.PB.Components = s.execPartition(cat, plan)

	// Remove unused PCLs while updating pclIndex. This can be done only after
	// components are updated to ensure components don't reference any unused CLs.
	pcls := make([]*prjpb.PCL, 0, len(s.PB.GetPcls())-len(cat.unused))
	for _, pcl := range s.PB.GetPcls() {
		id := common.CLID(pcl.GetClid())
		if cat.unused.Has(id) {
			delete(s.pclIndex, id)
		} else {
			s.pclIndex[id] = len(pcls)
			pcls = append(pcls, pcl)
		}
	}
	s.PB.Pcls = pcls
	s.PB.CreatedPruns = nil
	s.PB.RepartitionRequired = false
}

// planPartition returns a DisjointSet representing a partition of PCLs.
func (s *State) planPartition(cat *categorizedCLs) disjointset.DisjointSet {
	s.ensurePCLIndex()
	d := disjointset.New(len(s.PB.GetPcls())) // operates on indexes of PCLs

	// CLs of a Run must be in the same set.
	s.PB.IterIncompleteRuns(func(r *prjpb.PRun, _ *prjpb.Component) (stop bool) {
		clids := r.GetClids()
		first := s.pclIndex[common.CLID(clids[0])]
		for _, id := range clids[1:] {
			d.Merge(first, s.pclIndex[common.CLID(id)])
		}
		return
	})
	// Active deps of an active CL must be in the same set as the CL.
	for i, pcl := range s.PB.GetPcls() {
		if cat.active.HasI64(pcl.GetClid()) {
			for _, dep := range pcl.GetDeps() {
				id := dep.GetClid()
				if j, exists := s.pclIndex[common.CLID(id)]; exists && cat.active.HasI64(id) {
					d.Merge(i, j)
				}
			}
		}
	}
	return d
}

// execPartition returns components according to partition plan.
//
// Expects pclIndex to be same used by planPartition.
func (s *State) execPartition(cat *categorizedCLs, d disjointset.DisjointSet) []*prjpb.Component {
	canReuse := func(c *prjpb.Component) (root int, can bool) {
		// Old component can be re-used iff all of:
		//  (1) it has exactly the same set in the new partition.
		//  (2) it does not contain unused CLs.
		//  (3) it contains at least 1 active CL.
		clids := c.GetClids()

		// Check (1).
		root = d.RootOf(s.pclIndex[common.CLID(clids[0])])
		if d.SizeOf(root) != len(clids) {
			return -1, false
		}
		for _, id := range clids[1:] {
			if root != d.RootOf(s.pclIndex[common.CLID(id)]) {
				return -1, false
			}
		}

		// Check (2) and (3).
		hasActive := false
		for _, clid := range clids {
			if cat.unused.HasI64(clid) {
				if len(clids) != 1 {
					// Note that 2+ CL component which satisfies (1) can't have an unused
					// CL. Unused CL don't have relation to other CLs, and so wouldn't be
					// grouped into the same set in a new partition.
					panic(fmt.Errorf("component with %d CLs %v has unused CL %d", len(clids), clids, clid))
				}
				return -1, false
			}
			if cat.active.HasI64(clid) {
				hasActive = true
			}
		}
		if !hasActive {
			return -1, false
		}

		return root, true
	}
	// First, try to re-use existing components whenever possible.
	// Typically, this should cover most components.
	reused := make(map[int]*prjpb.Component, d.Count())
	var deleted []*prjpb.Component
	for _, c := range s.PB.GetComponents() {
		if root, yes := canReuse(c); yes {
			reused[root] = c
			continue
		}
		deleted = append(deleted, c)
	}

	// Now create new components.
	created := make(map[int]*prjpb.Component, d.Count()-len(reused))
	for index, pcl := range s.PB.GetPcls() {
		id := pcl.GetClid()
		if !cat.active.HasI64(id) {
			if size := d.SizeOf(index); size != 1 {
				panic(fmt.Errorf("inactive CLs must end up in a disjoint set of their own: %d => %d", id, size))
			}
			continue // no component for inactive CLs.
		}
		root := d.RootOf(index)
		if _, ok := reused[root]; ok {
			continue
		}
		c, exists := created[root]
		if !exists {
			size := d.SizeOf(root)
			c = &prjpb.Component{
				Clids:          make([]int64, 0, size),
				TriageRequired: true,
			}
			created[root] = c
		}
		c.Clids = append(c.GetClids(), id)
	}

	// And fill them with Incomplete runs.
	addPRuns := func(pruns []*prjpb.PRun) {
		for _, r := range pruns {
			index := s.pclIndex[common.CLID(r.GetClids()[0])]
			c := created[d.RootOf(index)]
			c.Pruns = append(c.GetPruns(), r) // not yet sorted order.
		}
	}
	addPRuns(s.PB.GetCreatedPruns())
	for _, c := range deleted {
		addPRuns(c.GetPruns())
	}

	out := make([]*prjpb.Component, 0, len(reused)+len(created))
	for _, c := range reused {
		out = append(out, c)
	}
	for _, c := range created {
		prjpb.SortPRuns(c.Pruns)
		out = append(out, c)
	}
	return out
}
