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
	"regexp"
	"sort"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// artifactConversionVersion identifies the version of artifact encoding format we're using.
const artifactConversionVersion = 1

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

// ArtifactsToByteSlices converts a slice of artifacts to a slice of byte slices.
// For each artifact, we reserve the leading byte to store conversion format version.
func ArtifactsToByteSlices(artifacts []*pb.Artifact) ([][]byte, error) {
	if len(artifacts) == 0 {
		return nil, nil
	}

	bytes := make([][]byte, len(artifacts))
	for i, art := range artifacts {
		buf := proto.NewBuffer([]byte{})
		if err := buf.EncodeVarint(artifactConversionVersion); err != nil {
			return nil, errors.Annotate(err, "artifact conversion version").Err()
		}
		if err := buf.EncodeMessage(art); err != nil {
			return nil, errors.Annotate(err, "converting artifact #%d %q", i, art.Name).Err()
		}
		bytes[i] = buf.Bytes()
	}
	return bytes, nil
}

// ArtifactsFromByteSlices unmarshals byte slices into a slice of pb.Artifacts.
// We reserve the leading byte in each slice to store conversion format version.
func ArtifactsFromByteSlices(byteSlices [][]byte) ([]*pb.Artifact, error) {
	arts := make([]*pb.Artifact, len(byteSlices))
	for i, b := range byteSlices {
		buf := proto.NewBuffer(b)

		// Check version.
		version, err := buf.DecodeVarint()
		if err != nil {
			return nil, errors.Annotate(err, "decoding version").Err()
		}
		if version != artifactConversionVersion {
			return nil, errors.Reason(
				"unrecognized artifact conversion version %d, expected %d",
				version, artifactConversionVersion).Err()
		}

		// Convert artifact bytes.
		arts[i] = &pb.Artifact{}
		if err := buf.DecodeMessage(arts[i]); err != nil {
			return nil, errors.Annotate(err, "converting byte slice #%d", i).Err()
		}
	}
	return arts, nil
}
