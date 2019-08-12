// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License")
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
	"bytes"
	"encoding/json"
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
)

var (
	// delim is the delimiter between directory/module components and leaf test name.
	// TODO: revisit given test path scheme.
	delim = "/"

	// requireFieldKeys are the keys required in test fields, so used to try conversion.
	requiredFieldKeys = []string{"actual", "expected"}
)

// JSONTestResults represents the structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Because the structure is in fact better defined than JSON unmarshalling
// permits, we read some values as "raw" values, to be converted individually.
//
// Deprecated fields are omitted.
type JSONTestResults struct {
	Interrupted bool

	NumFailuresByTypeRaw map[string]interface{} `json:"num_failures_by_Type"`
	NumFailuresByType    map[string]int

	PathDelimiter string `json:"path_delimiter"`

	SecondsSinceEpoch float64 `json:"seconds_since_epoch"`

	TestsRaw map[string]interface{} `json:"tests"`
	Tests    map[string]TestFields

	Version int32

	ArtifactTypesRaw          map[string]interface{} `json:"artifact_types"`
	ArtifactTypes             map[string]string
	ArtifactPermanentLocation string `json:"artifact_permanent_location"`

	BuildNumber string `json:"build_number"`
	BuilderName string `json:"builder_number"`

	Metadata map[string]interface{}

	ChromiumRevision string `json:"chromium_revision"`

	LayoutTestsDir string `json:"layout_tests_dir"`

	PixelTestsEnabled bool `json:"pixel_tests_enabled"`

	FlagName string `json:"flag_name"`
}

// TestFields represents the test fields structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Because the structure is in fact better defined than JSON unmarshalling
// permits, we read some values as "raw" values, to be converted individually.
//
// Deprecated fields are omitted.
type TestFields struct {
	Actual   string
	Expected string

	ArtifactsRaw map[string]interface{} `json:"artifacts"`
	Artifacts    map[string][]string

	Bugs string

	Shard int

	IsFlaky      bool `json:"is_flaky"`
	IsRegression bool `json:"is_regression"`
	IsUnexpected bool `json:"is_unexpected"`

	Time     float64
	TimesRaw []interface{} `json:"times"`
	Times    []float64

	HasRepaintOverlay bool   `json:"has_repaint_overlay"`
	IsMissingAudio    bool   `json:"is_missing_audio"`
	IsMissingTest     bool   `json:"is_missing_test"`
	IsMissingVideo    bool   `json:"is_missing_audio"`
	IsTestharnessTest bool   `json:"is_testharness_test"`
	ReftestType       string `json:"reftest_type"`
}

// ConvertFromJSON converts a JSON of test results in the JSON Test Results format to the internal struct format..
//
// TODO: convert to resultspb.Invocation.
func ConvertFromJSON(ctx context.Context, buf *bytes.Buffer) (*JSONTestResults, error) {
	results := &JSONTestResults{}
	err := json.Unmarshal(buf.Bytes(), results)
	if err != nil {
		return nil, err
	}

	// Convert Tests.
	for key := range results.TestsRaw {
		if err := convertTests(ctx, key, results.TestsRaw[key], results); err != nil {
			return nil, err
		}
	}

	// TODO: convert other raw fields.

	return results, nil
}

// convertTests converts the trie of tests.
func convertTests(ctx context.Context, testRoot string, curNode interface{}, results *JSONTestResults) error {
	// curNode must be a map[string]interface{}, whether it's a TestFields or an intermediate node.
	children, ok := curNode.(map[string]interface{})
	if !ok {
		return errors.Reason("failed to convert %v", curNode).Err()
	}

	// Otherwise, try to convert curNode to a TestFields first.
	if hasRequiredFieldKeys(children) {
		// curNode was created from Unmarshal, so Marshal should not err.
		fields := TestFields{}
		value, _ := json.Marshal(curNode)

		err := json.Unmarshal(value, &fields)
		if err != nil {
			return err
		}

		if results.Tests == nil {
			results.Tests = make(map[string]TestFields)
		}
		results.Tests[testRoot] = fields
		return nil
	}

	// Otherwise, couldn't convert to TestFields, so see if we can convert an intermediate node.
	for key := range children {
		convertTests(ctx, fmt.Sprintf("%s%s%s", testRoot, delim, key), children[key], results)
	}
	return nil
}

// hasRequiredFieldKeys checks for the keys required in test fields to determine if conversion is appropriate.
func hasRequiredFieldKeys(resultNode map[string]interface{}) bool {
	for _, key := range requiredFieldKeys {
		if _, ok := resultNode[key]; !ok {
			return false
		}
	}
	return true
}
