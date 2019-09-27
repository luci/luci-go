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
	"encoding/base64"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/results"

	rdb "go.chromium.org/luci/results"
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
}

// GTestRunResult represents the per_iteration_data as described in
// https://cs.chromium.org/chromium/src/base/test/launcher/test_results_tracker.h?l=83&rcl=96020cfd447cb285acfa1a96c37a67ed22fa2499
// (base::TestResultsTracker::SaveSummaryAsJSON)
//
// Fields not used by Test Results are omitted.
type GTestRunResult struct {
	Status        string `json:"status"`
	ElapsedTimeMs int    `json:"elapsed_time_ms"`

	LosslessSnippet     bool   `json:"losless_snippet"`
	OutputSnippetBase64 string `json:"output_snippet_base64"`
}

// Location describes a code location.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// ConvertFromJSON reads the provided reader into the receiver.
//
// The receiver is cleared and its fields overwritten.
func (r *GTestResults) ConvertFromJSON(ctx context.Context, reader io.Reader) error {
	*r = GTestResults{}
	if err := json.NewDecoder(reader).Decode(r); err != nil {
		return err
	}

	if len(r.AllTests) == 0 {
		return errors.Reason(`missing "all_tests" field in JSON`).Err()
	}

	return nil
}

// ToProtos converts test results in r []*resultspb.TestResult and updates inv
// in-place accordingly.
// If an error is returned, inv is left unchanged.
//
// Does not populate TestResult.Name.
func (r *GTestResults) ToProtos(ctx context.Context, req *resultspb.DeriveInvocationRequest, inv *resultspb.Invocation) ([]*resultspb.TestResult, error) {
	// In theory, we can have multiple iterations. This seems rare in practice, so log if we do see
	// more than one to confirm and track.
	if len(r.PerIterationData) > 1 {
		logging.Infof(ctx,
			"Got %d iterations for task %s on %s", len(r.PerIterationData), req.SwarmingTask.Id, req.SwarmingTask.Hostname)
	}

	// Assume the invocation was not interrupted; if any results are NOTRUN,
	// we'll mark as otherwise.
	interrupted := false

	var ret []*resultspb.TestResult
	var testNames []string
	for _, data := range r.PerIterationData {
		// Sort the test name to make the output deterministic.
		testNames = testNames[:0]
		for name := range data {
			testNames = append(testNames, name)
		}
		sort.Strings(testNames)

		for _, name := range testNames {
			baseName, params := extractGTestParameters(name)
			testPath := req.TestPathPrefix + baseName

			for i, result := range data[name] {
				// Store the processed test result into the correct part of the overall map.
				rpb, err := r.convertTestResult(ctx, testPath, name, result)
				if err != nil {
					return nil, errors.Annotate(err,
						"iteration %d of test %s failed to convert run result", i, name).Err()
				}

				if len(params) > 0 {
					rpb.ExtraVariantPairs = params.Proto()
				}

				// TODO(jchinlee): Verify that it's indeed the case that getting NOTRUN results in the final
				// results indicates the task was incomplete.
				// TODO(jchinlee): Check how unexpected SKIPPED tests should be handled.
				if result.Status == "NOTRUN" {
					interrupted = true
				}

				ret = append(ret, rpb)
			}
		}
	}

	// TODO(jchinlee): move this block of code to the callsite.
	// It is NOT specific to GTest.
	if err := results.VariantDefMap(req.BaseTestVariant.Def).Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid base test variant $q").Err()
	}
	inv.BaseTestVariantDef = req.BaseTestVariant

	// The code below does not return errors, so it is safe to make in-place
	// modifications of inv.

	if interrupted {
		inv.State = resultspb.Invocation_INTERRUPTED
	} else {
		inv.State = resultspb.Invocation_COMPLETED
	}

	// Populate the tags.
	for _, tag := range r.GlobalTags {
		inv.Tags = append(inv.Tags, util.StringPair("gtest_global_tag", tag))
	}
	inv.Tags = append(inv.Tags, util.StringPair("test_framework", "gtest"))

	rdb.NormalizeInvocation(inv)
	return ret, nil
}

func fromGTestStatus(s string) (resultspb.TestStatus, error) {
	switch s {
	case "SUCCESS":
		return resultspb.TestStatus_PASS, nil
	case "FAILURE":
		return resultspb.TestStatus_FAIL, nil
	case "FAILURE_ON_EXIT":
		return resultspb.TestStatus_FAIL, nil
	case "TIMEOUT":
		return resultspb.TestStatus_ABORT, nil
	case "CRASH":
		return resultspb.TestStatus_CRASH, nil
	case "SKIPPED":
		return resultspb.TestStatus_SKIP, nil
	case "EXCESSIVE_OUTPUT":
		return resultspb.TestStatus_FAIL, nil
	case "NOTRUN":
		return resultspb.TestStatus_SKIP, nil
	default:
		// This would only happen if the set of possible GTest result statuses change and resultsdb has
		// not been updated to match.
		return resultspb.TestStatus_STATUS_UNSPECIFIED, errors.Reason("unknown GTest status %q", s).Err()
	}
}

// extractGTestParameters extracts parameters from a test path as a mapping with "param/" keys.
func extractGTestParameters(testPath string) (basePath string, params rdb.VariantDefMap) {
	// TODO(jchinlee): Implement.
	basePath = testPath
	params = rdb.VariantDefMap{}
	return
}

func (r *GTestResults) convertTestResult(ctx context.Context, testPath, name string, result *GTestRunResult) (*resultspb.TestResult, error) {
	status, err := fromGTestStatus(result.Status)
	if err != nil {
		return nil, err
	}

	rpb := &resultspb.TestResult{
		TestPath: testPath,
		Status:   status,
		Tags: util.StringPairs(
			// Store the original GTest status.
			"gtest_status", result.Status,
			// Store the correct output snippet.
			"lossless_snippet", strconv.FormatBool(result.LosslessSnippet),
		),
	}

	// Do not set duration if it is unknown.
	if result.ElapsedTimeMs != 0 {
		rpb.Duration = secondsToDuration(1e-6 * float64(result.ElapsedTimeMs))
	}

	// Write the summary.
	if result.OutputSnippetBase64 != "" {
		outputBytes, err := base64.StdEncoding.DecodeString(result.OutputSnippetBase64)
		if err != nil {
			// Log the error, but we shouldn't fail to convert an entire invocation just because we can't
			// convert a summary.
			logging.Errorf(ctx, "Failed to convert OutputSnippetBase64 %q", result.OutputSnippetBase64)
		} else {
			// TODO(jchinlee): Escape Markdown.
			rpb.SummaryMarkdown = strings.ToValidUTF8(string(outputBytes), string(utf8.RuneError))
		}
	}

	// Store the test code location.
	if loc, ok := r.TestLocations[name]; ok {
		rpb.Tags = append(rpb.Tags,
			util.StringPair("gtest_file", loc.File),
			util.StringPair("gtest_line", strconv.Itoa(loc.Line)),
		)
	}

	return rpb, nil
}
