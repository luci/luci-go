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

package run

import (
	"sort"
)

// SortTryjobs sorts Run's tracked Tryjobs in-place.
func SortTryjobs(ts []*Tryjob) {
	sort.Slice(ts, func(i, j int) bool { return lessTryjob(ts[i], ts[j]) })
}

// DiffTryjobsForReport returns set-difference of `after`-`before`
// for reporting.
//
// `before` and `after` must be sorted sets of Run Tryjobs.
//
// When comparing prior and newer version of the same Run Tryjob,
// considers only the Status.
func DiffTryjobsForReport(before, after []*Tryjob) []*Tryjob {
	var out []*Tryjob
	for {
		switch {
		case len(before) == 0:
			return append(out, after...)
		case len(after) == 0:
			return out
		}
		switch b, a := before[0], after[0]; {
		case lessTryjob(b, a):
			before = before[1:]
		case lessTryjob(a, b):
			out = append(out, a)
			after = after[1:]
		case a.GetStatus() != b.GetStatus() || a.GetResult().GetStatus() != b.GetResult().GetStatus():
			out = append(out, a)
			before, after = before[1:], after[1:]
		default:
			// Same.
			before, after = before[1:], after[1:]
		}
	}
}

// lessTryjob is sort.Interface compatible Less function for Run's Tryjobs.
func lessTryjob(left, right *Tryjob) bool {
	switch {
	case left.GetId() < right.GetId():
		return true
	case left.GetId() > right.GetId():
		return false

		// TODO(crbug/1227523): delete after CQDaemon is deleted,
		// since all Run tryjobs will have CV IDs.
	case left.GetExternalId() < right.GetExternalId():
		return true
	case left.GetExternalId() > right.GetExternalId():
		return false

	default:
		return false
	}
}
