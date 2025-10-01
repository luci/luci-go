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

package inputbuffer

import "go.chromium.org/luci/common/errors"

// MergeOrderedRuns merges two sets of runs into a destination slice,
// assuming that both slices are sorted by commit position (oldest first), and
// then by result time (oldest first).
//
// The result will be a slice of runs which is pointers into the original
// aRuns and bRuns slices. Any modification to aRuns or bRuns will invalidate
// the result.
func MergeOrderedRuns(aRuns, bRuns []Run, dest *[]*Run) {
	if *dest == nil || cap(*dest) < len(aRuns)+len(bRuns) {
		*dest = make([]*Run, 0, len(aRuns)+len(bRuns))
	}
	// Reset destination slice to zero length.
	merged := (*dest)[:0]

	aPos := 0
	bPos := 0
	for aPos < len(aRuns) && bPos < len(bRuns) {
		cmp := compareRun(&aRuns[aPos], &bRuns[bPos])
		// Item in 'a' buffer is strictly older.
		if cmp == -1 {
			merged = append(merged, &aRuns[aPos])
			aPos++
		} else {
			merged = append(merged, &bRuns[bPos])
			bPos++
		}
	}

	// Add the remaining items.
	for ; aPos < len(aRuns); aPos++ {
		merged = append(merged, &aRuns[aPos])
	}
	for ; bPos < len(bRuns); bPos++ {
		merged = append(merged, &bRuns[bPos])
	}

	*dest = merged
}

func VerifyRunsOrdered(runs []*Run) error {
	for i, run := range runs {
		if i == 0 {
			continue
		}
		previousRun := runs[i-1]
		invariant := previousRun.SourcePosition < run.SourcePosition || (previousRun.SourcePosition == run.SourcePosition && !previousRun.Hour.After(run.Hour))
		if !invariant {
			return errors.Fmt("runs at index %v is out of order, (previous commit position %v, previous hour %v, current commit position %v, current hour %v)",
				i, previousRun.SourcePosition, previousRun.Hour, run.SourcePosition, run.Hour)
		}
	}
	return nil
}

// copyAndFlattenRuns copies the runs into a new []Run slice.
func copyAndFlattenRuns(rs []*Run) []Run {
	var result []Run
	for _, r := range rs {
		result = append(result, *r)
	}
	return result
}

// copyAndUnflattenRuns copies the runs into a new []*Run slice.
func copyAndUnflattenRuns(runs []Run) []*Run {
	result := make([]*Run, len(runs))

	// Instead of many small object allocations for
	// each individual run, make one large allocation.
	resultData := make([]Run, len(runs))
	for i, run := range runs {
		resultData[i] = run
		result[i] = &resultData[i]
	}
	return result
}
