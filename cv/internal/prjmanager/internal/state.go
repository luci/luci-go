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

package internal

import (
	"sort"

	"go.chromium.org/luci/cv/internal/common"
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
