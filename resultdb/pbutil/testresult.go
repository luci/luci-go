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
	"sort"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
// For each artifact, the first byte is reserved for conversion format version.
func ArtifactsToByteArrays(artifacts []*pb.Artifact) ([][]byte, error) {
	if len(artifacts) == 0 {
		return nil, nil
	}

	// Reserve first byte.
	bytes := make([][]byte, len(artifacts))
	for i, art := range artifacts {
		var err error
		if bytes[i], err = json.Marshal(art); err != nil {
			return nil, errors.Annotate(err, "converting artifact #%d %q", i, art.Name).Err()
		}

		bytes[i] = append(make([]byte, 1), bytes[i]...)
	}
	return bytes, nil
}

// ArtifactFromBytes unmarshals a byte array into a pb.Artifact.
// The first byte is expected to be conversion format version.
func ArtifactFromBytes(bytes []byte) (*pb.Artifact, error) {
	art := &pb.Artifact{}
	// Process everything after the first byte.
	if err := json.Unmarshal(bytes[1:], art); err != nil {
		return nil, err
	}
	return art, nil
}
