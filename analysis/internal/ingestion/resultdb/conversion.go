// Copyright 2022 The LUCI Authors.
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

package resultdb

import (
	"fmt"
	"sort"
	"strings"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// GroupAndOrderTestResults groups test results into test runs, and orders
// them by start time. Test results are returned in sorted start time order
// within the runs, and runs are ordered based on the start time of the first
// test result that is inside them.
// The result order is guaranteed to be deterministic even if all test
// results have the same start time.
func GroupAndOrderTestResults(input []*rdbpb.TestResultBundle) [][]*rdbpb.TestResultBundle {
	var result [][]*rdbpb.TestResultBundle
	runIndexByName := make(map[string]int)

	// Process results in order of StartTime.
	// This is to ensure test result indexes are later
	// assigned correctly w.r.t the actual execution order.
	input = sortResultsByStartTime(input)

	// Process test results, creating runs as they are needed.
	// Runs will be created in the order of the first test result
	// that is inside them.
	for _, tr := range input {
		testRun, err := InvocationFromTestResultName(tr.Result.Name)
		if err != nil {
			// This should never happen, as the test results came from
			// ResultDB.
			panic(err)
		}
		idx, ok := runIndexByName[testRun]
		if !ok {
			// Create an empty run.
			idx = len(result)
			runIndexByName[testRun] = idx
			result = append(result, nil)
		}

		result[idx] = append(result[idx], tr)
	}
	return result
}

// InvocationFromTestResultName extracts the invocation that the
// test result is immediately included inside.
func InvocationFromTestResultName(name string) (string, error) {
	// Using a regexp here was consuming 5% of all CPU cycles
	// related to test verdict ingestion, so do the extracting
	// manually using indexes.
	// The format of the name is
	// ^invocations/([^/]+)/tests/[^/]+/results/[^/]+$,
	// and we want to extract the invocation name.
	startIdx := strings.Index(name, "/")
	if startIdx < 0 || (startIdx+1) >= len(name) || name[:startIdx] != "invocations" {
		// This should never happen as the invocation came from ResultDB.
		return "", fmt.Errorf("invalid test result name %q, expected invocations/{invocation_name}/...", name)
	}
	endIdx := strings.Index(name[startIdx+1:], "/")
	if endIdx <= 0 {
		// This should never happen as the invocation came from ResultDB.
		return "", fmt.Errorf("invalid test result name %q, expected invocations/{invocation_name}/...", name)
	}
	endIdx = endIdx + (startIdx + 1)
	return name[startIdx+1 : endIdx], nil
}

func sortResultsByStartTime(results []*rdbpb.TestResultBundle) []*rdbpb.TestResultBundle {
	// Copy the results to avoid modifying parameter slice, which
	// the caller to IngestFromResultDB may not expect.
	sortedResults := make([]*rdbpb.TestResultBundle, len(results))
	for i, r := range results {
		sortedResults[i] = r
	}

	sort.Slice(sortedResults, func(i, j int) bool {
		aResult := sortedResults[i].Result
		bResult := sortedResults[j].Result
		aTime := aResult.StartTime.AsTime()
		bTime := bResult.StartTime.AsTime()
		if aTime.Equal(bTime) {
			// If start time the same, order by Result Name.
			// Needed to ensure the output of this sort is
			// deterministic given the input.
			return aResult.Name < bResult.Name
		}
		return aTime.Before(bTime)
	})
	return sortedResults
}
