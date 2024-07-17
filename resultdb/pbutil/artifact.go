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
	"regexp"

	"go.chromium.org/luci/common/errors"
)

const (
	// Unicode character classes L, M, N, P, S, Zs are the "graphic" type code points,
	// so a good approximation of all the printable characters.
	// Also accept '.' as the first character to enable upload of files starting with .
	artifactIDPattern       = `(?:[[:word:]]|\.)([\p{L}\p{M}\p{N}\p{P}\p{S}\p{Zs}]{0,508}[[:word:]])?`
	artifactIDPrefixPattern = `(?:[[:word:]]|\.)([\p{L}\p{M}\p{N}\p{P}\p{S}\p{Zs}]{0,509})?`
)

var (
	artifactIDRe                  = regexpf("^%s$", artifactIDPattern)
	artifactIDPrefixRe            = regexpf("^%s$", artifactIDPrefixPattern)
	invocationArtifactNamePattern = fmt.Sprintf("invocations/(%s)/artifacts/(.+)", invocationIDPattern)
	testResultArtifactNamePattern = fmt.Sprintf("invocations/(%s)/tests/([^/]+)/results/(%s)/artifacts/(.+)", invocationIDPattern, resultIDPattern)
	invocationArtifactNameRe      = regexpf("^%s$", invocationArtifactNamePattern)
	testResultArtifactNameRe      = regexpf("^%s$", testResultArtifactNamePattern)
	artifactNameRe                = regexpf("^%s|%s$", testResultArtifactNamePattern, invocationArtifactNamePattern)
	textArtifactContentTypeRe     = regexpf("^text/*")
)

// ValidateArtifactID returns a non-nil error if id is invalid.
func ValidateArtifactID(id string) error {
	return validateWithRe(artifactIDRe, id)
}

// ValidateArtifactIDPrefix returns a non-nil error if prefix is invalid.
func ValidateArtifactIDPrefix(idPrefix string) error {
	return validateWithRe(artifactIDPrefixRe, idPrefix)
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

	unescape := func(escaped string, re *regexp.Regexp) (string, error) {
		unescaped, err := url.PathUnescape(escaped)
		if err != nil {
			return "", errors.Annotate(err, "%q", escaped).Err()
		}

		if err := validateWithRe(re, unescaped); err != nil {
			return "", errors.Annotate(err, "%q", unescaped).Err()
		}

		return unescaped, nil
	}

	unescapeTestID := func(escaped string) (string, error) {
		unescaped, err := url.PathUnescape(escaped)
		if err != nil {
			return "", errors.Annotate(err, "%q", escaped).Err()
		}

		if err := ValidateTestID(unescaped); err != nil {
			return "", errors.Annotate(err, "%q", unescaped).Err()
		}

		return unescaped, nil
	}

	if m := invocationArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		artifactID, err = unescape(m[2], artifactIDRe)
		err = errors.Annotate(err, "artifact ID").Err()
		return
	}

	if m := testResultArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		if testID, err = unescapeTestID(m[2]); err != nil {
			err = errors.Annotate(err, "test ID").Err()
			return
		}
		resultID = m[3]
		artifactID, err = unescape(m[4], artifactIDRe)
		err = errors.Annotate(err, "artifact ID").Err()
		return
	}

	err = doesNotMatch(artifactNameRe)
	return
}

// InvocationArtifactName synthesizes a name of an invocation-level artifact.
// Does not validate IDs, use ValidateInvocationID and ValidateArtifactID.
func InvocationArtifactName(invocationID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/artifacts/%s", invocationID, url.PathEscape(artifactID))
}

// TestResultArtifactName synthesizes a name of an test-result-level artifact.
// Does not validate IDs, use ValidateInvocationID, ValidateTestID,
// ValidateResultID and ValidateArtifactID.
func TestResultArtifactName(invocationID, testID, resulID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/results/%s/artifacts/%s", invocationID, url.PathEscape(testID), resulID, url.PathEscape(artifactID))
}

// IsTextArtifact returns true if the content type represents a text-based artifact.
// Note: As the artifact content type field is optional, it is possible that
// a text artifact was uploaded to ResultDB without a content type. In such case,
// this function will return false.
//
// We rather miss some text artifact than wrongly classify a non-text artifact as
// text artifact.
func IsTextArtifact(contentType string) bool {
	return textArtifactContentTypeRe.MatchString(contentType)
}
