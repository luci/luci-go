// Copyright 2024 The LUCI Authors.
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

package changepoints

import (
	"math"
	"sort"
)

// TestIDGroupingThreshold is the threshold to partition changepoints by test ID.
// TODO: Set this threshold dynamically base on the total number of unique tests in the requested LUCI project and the number of regressions in this period.
// Because the significance of seeing a gap of 64 in test ID number depends on the above two factors.
// A possible formula to derive this threshold is (total tests / # of regressions in period) * coefficient.
const TestIDGroupingThreshold = 64

// RegressionRangeOverlapPrecThreshold decides whether two changepoints can be grouped together.
// The regression range overlap percentage is calculated by (# of overlapped commits/# of commits in the narrower regression range).
const RegressionRangeOverlapPrecThreshold = 0.4

// GroupChangepoints returns a 2D array where each row represents a group of changepoints.
// The grouping result is deterministic, which means same input always results in same groups.
// The groups are generated with the following steps.
//  1. partition changepoints base on test ID
//  2. For changepoints in each partition, group base on percentage regression range overlap.
func GroupChangepoints(rows []*ChangepointRow) [][]*ChangepointRow {
	testIDGroups := groupByTestID(rows)
	groups := [][]*ChangepointRow{}
	for _, testIDGroup := range testIDGroups {
		groupsForTestID := groupByRegressionRange(testIDGroup)
		groups = append(groups, groupsForTestID...)
	}
	return groups
}

// groupByTestID groups changepoints base on their test ID.
func groupByTestID(rows []*ChangepointRow) [][]*ChangepointRow {
	// Copy the input slice, so that we don't change the input.
	cps := make([]*ChangepointRow, len(rows))
	copy(cps, rows)
	sort.Slice(cps, func(i, j int) bool {
		return CompareTestVariantBranchChangepoint(cps[i], cps[j])
	})
	testIDGroups := [][]*ChangepointRow{}
	groupStart := 0
	// Iterate the sorted list of changepoints, and create a split between two adjacent changepoints
	// when the difference of their testIDNum is greater than TestIDGroupingThreshold.
	for groupEnd := 1; groupEnd < len(cps); groupEnd++ {
		if cps[groupEnd].TestIDNum-cps[groupEnd-1].TestIDNum > TestIDGroupingThreshold {
			testIDGroups = append(testIDGroups, cps[groupStart:groupEnd])
			groupStart = groupEnd
		}
	}
	testIDGroups = append(testIDGroups, cps[groupStart:])
	return testIDGroups
}

type testVariantKey struct {
	TestID      string
	VariantHash string
}

func toTestVariantKey(changepoint *ChangepointRow) testVariantKey {
	return testVariantKey{
		TestID:      changepoint.TestID,
		VariantHash: changepoint.VariantHash,
	}
}

// groupByRegressionRange groups changepoints base on overlap of 99% confidence interval of start position (aka. Regression range).
// The same test variant branch can only appears once in a group.
func groupByRegressionRange(rows []*ChangepointRow) [][]*ChangepointRow {
	// Copy the input slice, so that we don't change the input.
	cps := make([]*ChangepointRow, len(rows))
	copy(cps, rows)
	// Sort changepoints by regression range width ASC.
	// Regression range width is defined as start_position_upper_bound_99th - start_position_lower_bound_99th.
	// This is to avoid grouping small non-overlapping regressions together because of a base changepoint with large regression width.
	// For example,
	// Below is the regression range of 4 changepoints (cp = changepoint).
	// cp1 |-----------------------------------|
	//      cp2|------|  cp3|-----|  cp4|-----|
	// If cp1 is picked as the base changepoint, all 4 changepoints will be grouped together.
	// To avoid this, we should always pick changepoint with smaller regression width.
	sort.Slice(cps, func(i, j int) bool {
		wi := regressionRangeWidth(cps[i])
		wj := regressionRangeWidth(cps[j])
		if wi != wj {
			return wi < wj
		}
		// This is required to make sure the sort is deterministic when regression range width equal.
		return CompareTestVariantBranchChangepoint(cps[i], cps[j])
	})
	groups := [][]*ChangepointRow{}
	grouped := make([]bool, len(cps))
	for i := range cps {
		if grouped[i] {
			continue
		}
		// The first encountered ungrouped changepoint is picked as the base changepoint.
		// We find other ungrouped changepoints which has overlap greater than the threshold with the base changepoint,
		// and group them together with the base changepoint.
		// This implies that in each result group, all changepoints satisfy the overlap threshold with the base changepoint,
		// but a random pair of changepoints in a group might not satisfy the overlap threshold with each other.
		base := cps[i]
		// We record whether a test variant branch has been added to this group.
		// This is to avoid multiple changepoints from the same test variant branch being grouped into the same group.
		seenTestVariant := map[testVariantKey]bool{}
		group := []*ChangepointRow{base}
		seenTestVariant[toTestVariantKey(base)] = true
		for j := i + 1; j < len(cps); j++ {
			// Skip this changepoint when
			//   * it's already been grouped, OR
			//   * the test variant already exists in the group, OR
			//   * the changepoint happens on a different branch.
			if grouped[j] || seenTestVariant[toTestVariantKey(cps[j])] || base.RefHash != cps[j].RefHash {
				continue
			}
			overlap := numberOfOverlapCommit(base, cps[j])
			overlapPercentage := overlap / math.Min(float64(regressionRangeWidth(base)), float64(regressionRangeWidth(cps[j])))
			if overlapPercentage > RegressionRangeOverlapPrecThreshold {
				grouped[j] = true
				group = append(group, cps[j])
				seenTestVariant[toTestVariantKey(cps[j])] = true
			}
		}
		groups = append(groups, group)
	}
	return groups
}

func numberOfOverlapCommit(cp1, cp2 *ChangepointRow) float64 {
	return math.Min(float64(cp1.UpperBound99th), float64(cp2.UpperBound99th)) - math.Max(float64(cp1.LowerBound99th), float64(cp2.LowerBound99th)) + 1
}

func regressionRangeWidth(cp *ChangepointRow) int64 {
	return cp.UpperBound99th - cp.LowerBound99th + 1
}

// CompareTestVariantBranchChangepoint returns whether element at i is smaller than element at j
// by comparing TestIDNum, VariantHash, RefHash, NominalStartPosition.
func CompareTestVariantBranchChangepoint(cpi, cpj *ChangepointRow) bool {
	switch {
	case cpi.TestIDNum != cpj.TestIDNum:
		return cpi.TestIDNum < cpj.TestIDNum
	case cpi.VariantHash != cpj.VariantHash:
		return cpi.VariantHash < cpj.VariantHash
	case cpi.RefHash != cpj.RefHash:
		return cpi.RefHash < cpj.RefHash
	default:
		return cpi.NominalStartPosition < cpj.NominalStartPosition
	}
}
