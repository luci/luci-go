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

	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	rdb "go.chromium.org/luci/results"
	resultspb "go.chromium.org/luci/results/proto/v1"
	"go.chromium.org/luci/results/util"
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

// ToInvocation converts a JSONTestResults to a (partial) resultspb.Invocation.
func (r *JSONTestResults) ToInvocation(ctx context.Context, req *resultspb.DeriveInvocationFromSwarmingRequest) (*resultspb.Invocation, error) {
	if r.Version != 3 {
		return nil, errors.Reason("unknown JSON Test Results version %d", r.Version).Err()
	}

	// testsToVariants maps JSON Test Formatted-test base name to the variant IDs present to Results.
	// We store these maps to handle the same test and variants appearing in multiple iterations.
	testsToVariants := map[string]map[string][]*resultspb.TestResult{}
	variantDefs := map[string]*resultspb.VariantDef{}

	for name, fields := range r.Tests {
		baseName, params := extractJSONTestResultParameters(name)

		// Derive the variant definition from base variant and params.
		variantDefMap := rdb.MergeTestVariantMaps(req.BaseTestVariant.Def, params)
		if err := variantDefMap.Validate(); err != nil {
			return nil, errors.Annotate(err, "test %s failed to validate variant def", name).Err()
		}
		variantDef := variantDefMap.Proto()
		varID := variantDef.Digest
		if _, ok := variantDefs[varID]; !ok {
			variantDefs[varID] = variantDef
		}

		// Generate results associated with this test path.
		rpbs, err := fields.toProtos()
		if err != nil {
			return nil, errors.Annotate(err, "test %s failed to convert run fields", name).Err()
		}

		// Store the processed test result into the correct part of the overall map.
		if _, ok := testsToVariants[baseName]; !ok {
			testsToVariants[baseName] = map[string][]*resultspb.TestResult{}
		}
		testsToVariants[baseName][varID] = append(testsToVariants[baseName][varID], rpbs...)
	}

	// Now that we have all the results mapped by test path and variant, populate the invocation.
	inv := &resultspb.Invocation{
		Incomplete:  r.Interrupted,
		VariantDefs: variantDefs,
		Tags:        util.StringPairs("test_framework", "json"),
	}

	for baseName, variants := range testsToVariants {
		testpb := &resultspb.Invocation_Test{Path: req.TestPathPrefix + baseName}

		for id, results := range variants {
			testpb.Variants = append(testpb.Variants, &resultspb.Invocation_TestVariant{
				VariantId: id,
				Results:   results,
			})
		}

		inv.Tests = append(inv.Tests, testpb)
	}
	sort.Slice(inv.Tests, func(i, j int) bool { return inv.Tests[i].Path < inv.Tests[j].Path })

	// Get tags from metadata if any.
	tags, err := r.extractTags()
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		inv.Tags = append(inv.Tags, util.StringPair("json_format_tag", tag))
	}
	if r.BuildNumber != "" {
		inv.Tags = append(inv.Tags, util.StringPair("build_number", r.BuildNumber))
	}
	SortTagsInPlace(inv.Tags)

	return inv, nil
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
		// Try to convert the map values to a TestFields.
		maybeFields := &TestFields{}
		if err := json.Unmarshal(value, maybeFields); err != nil {
			return errors.Annotate(err, "%q not TestFields", value).Err()
		}

		// Set up test path.
		delim := "/"
		testPath := key
		if r.PathDelimiter != "" {
			delim = r.PathDelimiter
		}

		if curPath != "" {
			testPath = fmt.Sprintf("%s%s%s", curPath, delim, key)
		} else {
			if prefixJson, ok := r.Metadata[testNamePrefixKey]; ok {
				var prefix string
				if err := json.Unmarshal(prefixJson, &prefix); err != nil {
					return errors.Annotate(err, "%s not string, got %q", testNamePrefixKey, prefixJson).Err()
				}
				testPath = prefix + key
			}
		}

		// No error from unmarshalling to TestFields might mean that the RawMessage
		// was an arbitrary dict or intermediate node, so check that TestFields
		// required fields are populated.
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
		return nil, errors.Annotate(err, "tags not []string, got %v", maybeTags).Err()
	}

	return tags, nil
}

func ofJSONStatus(s string) (resultspb.Status, error) {
	switch s {
	case "CRASH":
		return resultspb.Status_CRASH, nil
	case "FAIL":
		return resultspb.Status_FAIL, nil
	case "PASS":
		return resultspb.Status_PASS, nil
	case "SKIP":
		return resultspb.Status_SKIP, nil
	case "TIMEOUT":
		return resultspb.Status_ABORT, nil
	default:
		// There are a number of web test-specific statuses not handled here as they are deprecated.
		return 0, errors.Reason("unknown or unexpected JSON Test Format status %s", s).Err()
	}
}

// extractJSONTestResultParameters extracts parameters from a test path as a mapping with "param/" keys.
func extractJSONTestResultParameters(testPath string) (basePath string, params rdb.VariantDefMap) {
	// TODO(jchinlee): Implement.
	basePath = testPath
	params = rdb.VariantDefMap{}
	return
}

// toProtos converts the TestFields into a slice of resultspb.TestResult.
func (f *TestFields) toProtos() ([]*resultspb.TestResult, error) {
	// Process statuses.
	actualStatuses := strings.Split(f.Actual, " ")
	expectedSet := stringset.NewFromSlice(strings.Split(f.Expected, " ")...)

	// Process times.
	// Durations are optional, but if they exist, the number should match that of runs.
	durations := make([]float64, 0, len(f.Times)+1)
	if len(f.Times) > 0 {
		durations = f.Times
	} else {
		durations = []float64{f.Time}
	}

	// Times are optional, but there should be one per run if any.
	if len(durations) > 0 && len(actualStatuses) != len(durations) {
		return nil, errors.Reason(
			"%d durations populated but has %d test statuses; should match",
			len(durations), len(actualStatuses)).Err()
	}

	rpbs := make([]*resultspb.TestResult, len(actualStatuses))
	for i, runStatus := range actualStatuses {
		status, err := ofJSONStatus(runStatus)
		if err != nil {
			return nil, err
		}

		rpbs[i] = &resultspb.TestResult{
			Expected: &wrappers.BoolValue{
				Value: expectedSet.Has(runStatus),
			},
			Status: status,
			Tags:   util.StringPairs("json_format_status", runStatus),
		}

		if len(durations) > 0 {
			rpbs[i].Duration = secondsToDuration(durations[i])
		}

		// TODO(jchinlee): Populate artifacts.
	}

	return rpbs, nil
}
