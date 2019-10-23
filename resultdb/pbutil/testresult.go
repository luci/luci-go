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

package pbutil

import (
	"encoding/json"
	"regexp"
	"sort"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var testPathRe = regexp.MustCompile(`^[[:print:]]+$`)

// ValidateTestPath returns a non-nil error if testPath is invalid.
func ValidateTestPath(testPath string) error {
	if testPath == "" {
		return unspecified()
	}
	if !testPathRe.MatchString(testPath) {
		return doesNotMatch(testPathRe)
	}
	return nil
}

// NormalizeTestResult converts inv to the canonical form.
func NormalizeTestResult(tr *pb.TestResult) {
	sortStringPairs(tr.Tags)
}

// NormalizeTestResultSlice converts trs to the canonical form.
func NormalizeTestResultSlice(trs []*pb.TestResult) {
	for _, tr := range trs {
		NormalizeTestResult(tr)
	}
	sort.Slice(trs, func(i, j int) bool {
		a := trs[i]
		b := trs[j]
		if a.TestPath != b.TestPath {
			return a.TestPath < b.TestPath
		}
		return a.Name < b.Name
	})
}

// ArtifactsToByteArrays converts a slice of artifacts to a slice of byte arrays.
func ArtifactsToByteArrays(artifacts []*pb.Artifact) ([][]byte, error) {
	if len(artifacts) == 0 {
		return nil, nil
	}

	bytes := make([][]byte, len(artifacts))
	for i, art := range artifacts {
		var err error
		if bytes[i], err = json.Marshal(art); err != nil {
			return nil, errors.Annotate(err, "converting artifact #%d %q", i, art.Name).Err()
		}
	}
	return bytes, nil
}
