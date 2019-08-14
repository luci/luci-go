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

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
)

// JSONTestResults represents the structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Deprecated fields and fields not used by Test Results are omitted.
type JSONTestResults struct {
	Interrupted bool `json:"interrupted"`

	PathDelimiter string `json:"path_delimiter"`

	SecondsSinceEpoch float64 `json:"seconds_since_epoch"`

	TestsRaw json.RawMessage `json:"tests"`
	Tests    map[string]*TestFields

	Version int32 `json:"version"`

	ArtifactTypes map[string]string `json:"artifact_types"`

	BuildNumber string `json:"build_number"`
	BuilderName string `json:"builder_number"`
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
// TODO: convert to resultspb.Invocation.
func ConvertFromJSON(ctx context.Context, reader io.Reader) (*JSONTestResults, error) {
	results := &JSONTestResults{}
	if err := json.NewDecoder(reader).Decode(results); err != nil {
		return nil, err
	}

	// Convert Tests and return.
	if err := results.convertTests("", results.TestsRaw); err != nil {
		return nil, err
	}
	return results, nil
}

// convertTests converts the trie of tests.
func (r *JSONTestResults) convertTests(curPath string, curNode json.RawMessage) error {
	// curNode should certainly be a map.
	var maybeNode map[string]json.RawMessage
	if err := json.Unmarshal(curNode, &maybeNode); err != nil {
		return errors.Annotate(err, "%v not map[string]json.RawMessage", curNode).Err()
	}

	// Convert the tree.
	for key, value := range maybeNode {
		// Try to convert the map values to a TestFields.
		maybeFields := &TestFields{}
		if err := json.Unmarshal(value, maybeFields); err != nil {
			return errors.Annotate(err, "%v not TestFields", value).Err()
		}

		// Set up test path.
		delim := "/"
		testPath := key
		if r.PathDelimiter != "" {
			delim = r.PathDelimiter
		}
		if curPath != "" {
			testPath = fmt.Sprintf("%s%s%s", curPath, delim, key)
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
			return errors.Annotate(err, "error attempting conversion of %v as intermediated node", value).Err()
		}
	}

	return nil
}
