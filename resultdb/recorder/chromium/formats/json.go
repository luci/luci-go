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
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/util"
)

const testNamePrefixKey = "test_name_prefix"

// JSONTestResults represents the structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Deprecated fields and fields not used by Test Results are omitted.
type JSONTestResults struct {
	Interrupted bool `json:"interrupted"`

	PathDelimiter string `json:"path_delimiter"`

	TestsRaw json.RawMessage `json:"tests"`
	Tests    map[string]*TestFields

	Version int32 `json:"version"`

	ArtifactTypes map[string]string `json:"artifact_types"`

	BuildNumber string `json:"build_number"`
	BuilderName string `json:"builder_name"`

	// Metadata associated with results, which may include a list of expectation_files, or
	// test_name_prefix e.g. in GPU tests (distinct from test_path_prefix passed in the recorder API
	// request).
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// TestFields represents the test fields structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Deprecated fields and fields not used by Test Results are omitted.
type TestFields struct {
	Actual   string `json:"actual"`
	Expected string `json:"expected"`

	Artifacts map[string][]string `json:"artifacts"`

	Time  float64   `json:"time"`
	Times []float64 `json:"times"`
}

// ConvertFromJSON converts a JSON of test results in the JSON Test Results
// format to the internal struct format.
//
// The receiver is cleared and its fields overwritten.
func (r *JSONTestResults) ConvertFromJSON(ctx context.Context, reader io.Reader) error {
	*r = JSONTestResults{}
	if err := json.NewDecoder(reader).Decode(r); err != nil {
		return err
	}

	// Convert Tests and return.
	if err := r.convertTests("", r.TestsRaw); err != nil {
		return err
	}
	return nil
}

// ToProtos converts test results in r []*resultspb.TestResult and updates inv
// in-place accordingly.
// If an error is returned, inv is left unchanged.
//
// Does not populate TestResult.Name.
func (r *JSONTestResults) ToProtos(ctx context.Context, req *pb.DeriveInvocationRequest, inv *pb.Invocation) ([]*pb.TestResult, error) {
	if r.Version != 3 {
		return nil, errors.Reason("unknown JSON Test Results version %d", r.Version).Err()
	}

	// Sort the test name to make the output deterministic.
	testNames := make([]string, 0, len(r.Tests))
	for name := range r.Tests {
		testNames = append(testNames, name)
	}
	sort.Strings(testNames)

	ret := make([]*pb.TestResult, 0, len(r.Tests))
	for _, name := range testNames {
		testPath := req.TestPathPrefix + name
		if err := r.Tests[name].toProtos(&ret, testPath); err != nil {
			return nil, errors.Annotate(err, "test %q failed to convert run fields", name).Err()
		}
	}

	// Get tags from metadata if any.
	tags, err := r.extractTags()
	if err != nil {
		return nil, err
	}

	// TODO(jchinlee): move this block of code to the callsite.
	// It is NOT specific to GTest.
	if err := resultdb.VariantDefMap(req.BaseTestVariant.Def).Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid base test variant $q").Err()
	}
	inv.BaseTestVariantDef = req.BaseTestVariant

	// The code below does not return errors, so it is safe to make in-place
	// modifications of inv.

	if r.Interrupted {
		inv.State = pb.Invocation_INTERRUPTED
	} else {
		inv.State = pb.Invocation_COMPLETED
	}

	inv.Tags = append(inv.Tags, util.StringPair("test_framework", "json"))
	for _, tag := range tags {
		inv.Tags = append(inv.Tags, util.StringPair("json_format_tag", tag))
	}
	if r.BuildNumber != "" {
		inv.Tags = append(inv.Tags, util.StringPair("build_number", r.BuildNumber))
	}

	resultdb.NormalizeInvocation(inv)
	return ret, nil
}

// convertTests converts the trie of tests.
func (r *JSONTestResults) convertTests(curPath string, curNode json.RawMessage) error {
	// curNode should certainly be a map.
	var maybeNode map[string]json.RawMessage
	if err := json.Unmarshal(curNode, &maybeNode); err != nil {
		return errors.Annotate(err, "%q not map[string]json.RawMessage", curNode).Err()
	}

	// Convert the tree.
	for key, value := range maybeNode {
		// Set up test path.
		delim := "/"
		testPath := key
		if r.PathDelimiter != "" {
			delim = r.PathDelimiter
		}

		if curPath != "" {
			testPath = fmt.Sprintf("%s%s%s", curPath, delim, key)
		} else {
			if prefixJSON, ok := r.Metadata[testNamePrefixKey]; ok {
				var prefix string
				if err := json.Unmarshal(prefixJSON, &prefix); err != nil {
					return errors.Annotate(err, "%s not string, got %q", testNamePrefixKey, prefixJSON).Err()
				}
				testPath = prefix + key
			}
		}

		// Try to unmarshal value to TestFields. We check success by checking fields we expect to
		// be populated.
		maybeFields := &TestFields{}
		json.Unmarshal(value, maybeFields)
		if maybeFields.Actual != "" && maybeFields.Expected != "" {
			if r.Tests == nil {
				r.Tests = make(map[string]*TestFields)
			}
			r.Tests[testPath] = maybeFields
			continue
		}

		// Otherwise, try to process it as an intermediate node.
		if err := r.convertTests(testPath, value); err != nil {
			return errors.Annotate(err, "error attempting conversion of %q as intermediated node", value).Err()
		}
	}
	return nil
}

// extractTags tries to read the optional "tags" field in "metadata" as a slice of strings.
func (r *JSONTestResults) extractTags() ([]string, error) {
	maybeTags, ok := r.Metadata["tags"]
	if !ok {
		return nil, nil
	}

	var tags []string
	if err := json.Unmarshal(maybeTags, &tags); err != nil {
		return nil, errors.Annotate(err, "tags not []string, got %q", maybeTags).Err()
	}

	return tags, nil
}

func fromJSONStatus(s string) (pb.TestStatus, error) {
	switch s {
	case "CRASH":
		return pb.TestStatus_CRASH, nil
	case "FAIL":
		return pb.TestStatus_FAIL, nil
	case "PASS":
		return pb.TestStatus_PASS, nil
	case "SKIP":
		return pb.TestStatus_SKIP, nil
	case "TIMEOUT":
		return pb.TestStatus_ABORT, nil

	// The below are web test-specific statuses. They are officially deprecated, but in practice
	// still generated by the tests and should be converted.
	case "IMAGE", "TEXT", "IMAGE+TEXT", "AUDIO", "LEAK", "MISSING":
		return pb.TestStatus_FAIL, nil

	default:
		// There are a number of web test-specific statuses not handled here as they are deprecated.
		return 0, errors.Reason("unknown or unexpected JSON Test Format status %s", s).Err()
	}
}

// toProtos converts the TestFields into zero or more resultspb.TestResult and
// appends them to dest.
func (f *TestFields) toProtos(dest *[]*pb.TestResult, testPath string) error {
	// Process statuses.
	actualStatuses := strings.Split(f.Actual, " ")
	expectedSet := stringset.NewFromSlice(strings.Split(f.Expected, " ")...)

	// Process times.
	// Time and Times are both optional, but if Times is present, its length should match the number
	// of runs. Otherwise we have only Time as the duration of the first run.
	if len(f.Times) > 0 && len(f.Times) != len(actualStatuses) {
		return errors.Reason(
			"%d durations populated but has %d test statuses; should match",
			len(f.Times), len(actualStatuses)).Err()
	}

	var durations []float64
	if len(f.Times) > 0 {
		durations = f.Times
	} else if f.Time != 0 { // Do not set duration if it is unknown.
		durations = []float64{f.Time}
	}

	// Populate protos.
	for i, runStatus := range actualStatuses {
		status, err := fromJSONStatus(runStatus)
		if err != nil {
			return err
		}

		tr := &pb.TestResult{
			TestPath: testPath,
			Expected: expectedSet.Has(runStatus),
			Status:   status,
			Tags:     util.StringPairs("json_format_status", runStatus),
		}

		if i < len(durations) {
			tr.Duration = secondsToDuration(durations[i])
		}

		// TODO(jchinlee): Populate artifacts.

		*dest = append(*dest, tr)
	}

	return nil
}
