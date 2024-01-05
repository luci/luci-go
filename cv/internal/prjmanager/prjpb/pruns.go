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

package prjpb

import (
	"sort"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/copyonwrite"
	"go.chromium.org/luci/cv/internal/run"
)

// IncompleteRuns are IDs of Runs which aren't yet completed.
func (p *PState) IncompleteRuns() (ids common.RunIDs) {
	p.IterIncompleteRuns(func(r *PRun, _ *Component) bool {
		ids = append(ids, common.RunID(r.GetId()))
		return false
	})
	sort.Sort(ids)
	return ids
}

// IterIncompleteRuns executes callback on each tracked Run.
//
// Callback is given a Component if Run is assigned to a component, or a nil
// Component if Run is part of `.CreatedRuns` not yet assigned to any component.
// Stops iteration if callback returns true.
func (p *PState) IterIncompleteRuns(callback func(r *PRun, c *Component) (stop bool)) {
	for _, c := range p.GetComponents() {
		for _, r := range c.GetPruns() {
			if callback(r, c) {
				return
			}
		}
	}
	for _, r := range p.GetCreatedPruns() {
		if callback(r, nil) {
			return
		}
	}
}

// MakePRun creates a PRun given a Run.
func MakePRun(r *run.Run) *PRun {
	clids := common.CLIDsAsInt64s(r.CLs)
	sort.Sort(sortableInt64s(clids))
	return &PRun{
		Id:       string(r.ID),
		Clids:    clids,
		Mode:     string(r.Mode),
		RootClid: int64(r.RootCL),
	}
}

type sortableInt64s []int64

func (s sortableInt64s) Len() int               { return len(s) }
func (s sortableInt64s) Less(i int, j int) bool { return s[i] < s[j] }
func (s sortableInt64s) Swap(i int, j int)      { s[i], s[j] = s[j], s[i] }

// COWCreatedRuns copy-on-write modifies CreatedRuns.
func (p *PState) COWCreatedRuns(m func(*PRun) *PRun, toAdd []*PRun) ([]*PRun, bool) {
	in := cowPRuns(p.GetCreatedPruns())
	out, updated := copyonwrite.Update(in, runModifier(m), cowPRuns(toAdd))
	return []*PRun(out.(cowPRuns)), updated
}

// COWPRuns copy-on-write modifies component's runs.
func (component *Component) COWPRuns(m func(*PRun) *PRun, toAdd []*PRun) ([]*PRun, bool) {
	in := cowPRuns(component.GetPruns())
	out, updated := copyonwrite.Update(in, runModifier(m), cowPRuns(toAdd))
	return []*PRun(out.(cowPRuns)), updated
}

func runModifier(f func(*PRun) *PRun) copyonwrite.Modifier {
	if f == nil {
		return nil
	}
	return func(v any) any {
		if v := f(v.(*PRun)); v != nil {
			return v
		}
		return copyonwrite.Deletion
	}
}

func SortPRuns(pruns []*PRun) {
	sort.Sort(cowPRuns(pruns))
}

type cowPRuns []*PRun

// It's important that PRuns are always sorted.
var _ copyonwrite.SortedSlice = cowPRuns(nil)

func (c cowPRuns) At(index int) any {
	return c[index]
}

func (c cowPRuns) Append(v any) copyonwrite.Slice {
	return append(c, v.(*PRun))
}

func (c cowPRuns) CloneShallow(length int, capacity int) copyonwrite.Slice {
	r := make(cowPRuns, length, capacity)
	copy(r, c[:length])
	return r
}

func (c cowPRuns) LessElements(a any, b any) bool {
	return a.(*PRun).GetId() < b.(*PRun).GetId()
}

func (c cowPRuns) Len() int               { return len(c) }
func (c cowPRuns) Less(i int, j int) bool { return c[i].GetId() < c[j].GetId() }
func (c cowPRuns) Swap(i int, j int)      { c[i], c[j] = c[j], c[i] }
