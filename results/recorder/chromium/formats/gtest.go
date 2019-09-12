// Copyright 2019 The LUCI Authors.
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

package formats

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	resultspb "go.chromium.org/luci/results/proto/v1"
	"go.chromium.org/luci/results/util"
)

// GTestResults represents the structure as described to be generated in
// https://cs.chromium.org/chromium/src/base/test/launcher/test_results_tracker.h?l=83&rcl=96020cfd447cb285acfa1a96c37a67ed22fa2499
// (base::TestResultsTracker::SaveSummaryAsJSON)
//
// Fields not used by Test Results are omitted.
type GTestResults struct {
	AllTests   []string `json:"all_tests"`
	GlobalTags []string `json:"global_tags"`

	// PerIterationData is a vector of run iterations, each mapping test names to a list of test data.
	PerIterationData []map[string][]*GTestRunResult `json:"per_iteration_data"`

	// TestLocations maps test names to their location in code.
	TestLocations map[string]*Location `json:"test_locations"`

	// StartTime is the start (epoch) time in seconds for the containing swarming shard.
	// GTest results don't have start times, only elapsed times, so we estimate.
	StartTime float64
}

// GTestRunResult represents the per_iteration_data as described in
// https://cs.chromium.org/chromium/src/base/test/launcher/test_results_tracker.h?l=83&rcl=96020cfd447cb285acfa1a96c37a67ed22fa2499
// (base::TestResultsTracker::SaveSummaryAsJSON)
//
// Fields not used by Test Results are omitted.
type GTestRunResult struct {
	Status        string `json:"status"`
	ElapsedTimeMs int    `json:"elapsed_time_ms"`

	OutputSnippet       string `json:"output_snippet"`
	LosslessSnippet     bool   `json:"losless_snippet"`
	OutputSnippetBase64 string `json:"output_snippet_base64"`
}

// Location describes a code location.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// ConvertFromJSON reads the provided reader into the receiver.
func (r *GTestResults) ConvertFromJSON(ctx context.Context, reader io.Reader) error {
	if err := json.NewDecoder(reader).Decode(r); err != nil {
		return err
	}

	if len(r.AllTests) == 0 {
		return errors.Reason(`missing "all_tests" field in JSON`).Err()
	}

	return nil
}

// ToInvocation converts a JSONTestResults to a (partial) resultspb.Invocation.
func (r *GTestResults) ToInvocation(ctx context.Context, req *resultspb.DeriveInvocationFromSwarmingRequest) (*resultspb.Invocation, error) {
	inv := &resultspb.Invocation{}

	// testsToVariants maps GTest base name to the variant IDs present to Results.
	// We store these maps to handle the same test and variants appearing in multiple iterations.
	testsToVariants := map[string]map[string][]*resultspb.Result{}
	variantDefs := map[string]*resultspb.VariantDef{}

	// curTime tracks a start time for the test run. It is a lie, as not even order is guaranteed to
	// match, but test runs have only elapsed time so we use the swarming task start time to derive a
	// test run start and end.
	curTime := r.StartTime

	// In theory, we can have multiple iterations. This seems rare in practice, so log if we do see
	// more than one to confirm and track.
	if len(r.PerIterationData) > 1 {
		logging.Infof(ctx,
			"Got %d iterations for task %s on %s", len(r.PerIterationData), req.Task.Id, req.Task.Hostname)
	}

	for _, data := range r.PerIterationData {
		for name, results := range data {
			baseName, params := r.getParameters(name)

			// Derive the variant definition from base variant and params.
			variantDef := r.getTestVariant(req.BaseTestVariant.Def, params)
			varID := VariantDefMap(variantDef.Def).GetID()
			if _, ok := variantDefs[varID]; !ok {
				variantDefs[varID] = variantDef
			}

			// Generate individual result protos.
			rpbs := make([]*resultspb.Result, len(results))
			for i, result := range results {
				rpb := &resultspb.Result{
					Status:  r.getStatus(result.Status),
					Summary: &resultspb.Markdown{Text: result.OutputSnippetBase64},
				}

				// TODO: Verify that it's indeed the case that getting NOTRUN results in the final results
				// indicates the task was incomplete.
				if result.Status == "NOTRUN" {
					inv.Incomplete = true
				}

				// Approximate times.
				rpb.StartTime = secondsToTimestamp(curTime)
				curTime += 1e-6 * float64(result.ElapsedTimeMs)
				rpb.EndTime = secondsToTimestamp(curTime)

				// Store the correct output snippet.
				rpb.Tags = append(
					rpb.Tags,
					util.StringPair("lossless_snippet", strconv.FormatBool(result.LosslessSnippet)),
				)
				if result.LosslessSnippet {
					rpb.Summary.Text = result.OutputSnippet
				}

				// Store the test code location.
				if loc, ok := r.TestLocations[name]; ok {
					rpb.Tags = append(rpb.Tags,
						util.StringPair("file", loc.File),
						util.StringPair("line", strconv.Itoa(loc.Line)),
					)
				}

				rpbs[i] = rpb
			}

			// Store the processed test result into the correct part of the overall map.
			testPath := fmt.Sprintf("%s%s", req.TestPathPrefix, baseName)
			if _, ok := testsToVariants[testPath]; !ok {
				testsToVariants[testPath] = map[string][]*resultspb.Result{}
			}
			testsToVariants[testPath][varID] = append(testsToVariants[testPath][varID], rpbs...)
		}
	}

	// Now that we have all the results mapped by test names and variant, populate the invocation.
	for name, variants := range testsToVariants {
		testpb := &resultspb.Invocation_Test{Name: name}

		for id, results := range variants {
			testpb.Variants = append(testpb.Variants, &resultspb.Invocation_TestVariant{
				VariantId: id,
				Results:   results,
			})
		}

		inv.Tests = append(inv.Tests, testpb)
	}

	inv.VariantDefs = variantDefs

	// Populate the tags.
	for _, tag := range r.GlobalTags {
		inv.Tags = append(inv.Tags, util.StringPair("global_tag", tag))
	}

	return inv, nil
}

func (r *GTestResults) getStatus(status string) resultspb.Status {
	switch status {
	case "SUCCESS":
		return resultspb.Status_PASS
	case "FAILURE":
		return resultspb.Status_FAIL
	case "FAILURE_ON_EXIT":
		return resultspb.Status_FAIL
	case "CRASH":
		return resultspb.Status_CRASH
	case "TIMEOUT":
		return resultspb.Status_ABORT
	case "SKIPPED":
		return resultspb.Status_SKIP
	case "EXCESSIVE_OUTPUT":
		return resultspb.Status_PASS
	case "NOTRUN":
		return resultspb.Status_SKIP
	}
	return resultspb.Status_STATUS_UNSPECIFIED
}

// getParameters gets parameters from a test path as mapping with "param/" keys.
func (r *GTestResults) getParameters(testPath string) (basePath string, params VariantDefMap) {
	// TODO: Implement.
	basePath = testPath
	params = VariantDefMap{}
	return
}

// getTestVariant gets the test variant def from merging the input maps.
//
// If multiple maps define the same key, the according value mapped in the last map of the arguments
// overwrites the previous values.
func (r *GTestResults) getTestVariant(maps ...map[string]string) *resultspb.VariantDef {
	def := VariantDefMap{}
	var keys []string

	for _, m := range maps {
		for k, v := range m {
			def[k] = v
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return def.GetProto(keys)
}
