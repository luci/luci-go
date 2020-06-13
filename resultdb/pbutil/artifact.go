// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"net/url"

	"go.chromium.org/luci/common/errors"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

const (
	artifactIDPattern = `[[:word:]]([[:print:]]{0,254}[[:word:]])?`
)

var (
	artifactIDRe                  = regexpf("^%s$", artifactIDPattern)
	invocationArtifactNamePattern = fmt.Sprintf("invocations/(%s)/artifacts/(%s)", invocationIDPattern, artifactIDPattern)
	testResultArtifactNamePattern = fmt.Sprintf("invocations/(%s)/tests/([^/]+)/results/(%s)/artifacts/(%s)", invocationIDPattern, resultIDPattern, artifactIDPattern)
	invocationArtifactNameRe      = regexpf("^%s$", invocationArtifactNamePattern)
	testResultArtifactNameRe      = regexpf("^%s$", testResultArtifactNamePattern)
	artifactNameRe                = regexpf("^%s|%s$", testResultArtifactNamePattern, invocationArtifactNamePattern)
)

// ValidateArtifactID returns a non-nil error if id is invalid.
func ValidateArtifactID(id string) error {
	return validateWithRe(artifactIDRe, id)
}

// ValidateArtifactName returns a non-nil error if name is invalid.
func ValidateArtifactName(name string) error {
	return validateWithRe(artifactNameRe, name)
}

// ParseArtifactName extracts the invocation ID, unescaped test id, result ID
// and artifact ID.
// The testID and resultID are empty if this is an invocation-level artifact.
func ParseArtifactName(name string) (invocationID, testID, resultID, artifactID string, err error) {
	if name == "" {
		err = unspecified()
		return
	}

	if m := invocationArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		artifactID = m[2]
		return
	}

	if m := testResultArtifactNameRe.FindStringSubmatch(name); m != nil {
		testID, err = url.PathUnescape(m[2])
		if err != nil {
			err = errors.Annotate(err, "test id %q", m[2]).Err()
			return
		}

		if ve := validateWithRe(testIDRe, testID); ve != nil {
			err = errors.Annotate(ve, "test id %q", testID).Err()
			return
		}

		invocationID = m[1]
		resultID = m[3]
		artifactID = m[4]
		return
	}

	err = doesNotMatch(artifactNameRe)
	return
}

// InvocationArtifactName synthesizes a name of an invocation-level artifact.
// Does not validate IDs, use ValidateInvocationID and ValidateArtifactID.
func InvocationArtifactName(invocationID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/artifacts/%s", invocationID, artifactID)
}

// TestResultArtifactName synthesizes a name of an test-result-level artifact.
// Does not validate IDs, use ValidateInvocationID, ValidateTestID,
// ValidateResultID and ValidateArtifactID.
func TestResultArtifactName(invocationID, testID, resulID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/results/%s/artifacts/%s", invocationID, url.PathEscape(testID), resulID, artifactID)
}

// ValidateSinkArtifact returns a non-nil error if art is invalid.
func ValidateSinkArtifact(art *sinkpb.Artifact) error {
	if art.GetFilePath() == "" && art.GetContents() == nil {
		return errors.Reason("body: either file_path or contents must be provided").Err()
	}
	// TODO(1017288) - validate content_type
	// skip `ContentType`
	return nil
}

// ValidateSinkArtifacts returns a non-nil error if any element of arts is invalid.
func ValidateSinkArtifacts(arts map[string]*sinkpb.Artifact) error {
	for id, art := range arts {
		if art == nil {
			return errors.Reason("%s: %s", id, unspecified()).Err()
		}
		if err := ValidateArtifactID(id); err != nil {
			return errors.Annotate(err, "%s", id).Err()
		}
		if err := ValidateSinkArtifact(art); err != nil {
			return errors.Annotate(err, "%s", id).Err()
		}
	}
	return nil
}
