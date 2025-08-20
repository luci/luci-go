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
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"
)

const (
	// Unicode character classes L, M, N, P, S, Zs are the "graphic" type code points,
	// so a good approximation of all the printable characters.
	// Also accept '.' as the first character to enable upload of files starting with .
	artifactIDPattern       = `(?:[[:word:]]|\.)([\p{L}\p{M}\p{N}\p{P}\p{S}\p{Zs}]{0,508}[[:word:]])?`
	artifactIDPrefixPattern = `(?:[[:word:]]|\.)([\p{L}\p{M}\p{N}\p{P}\p{S}\p{Zs}]{0,509})?`
	// The RBE URI format: "bytestream://<HOSTNAME>/projects/<PROJECT_ID>/instances/<INSTANCE_ID>/blobs/<HASH>/<SIZE_BYTES>".
	// Captures: 1=PROJECT_ID, 2=INSTANCE_ID, 3=HASH, 4=SIZE_BYTES
	rbeURIPatten = `bytestream://[^/]+/projects/([^/]+)/instances/([^/]+)/blobs/([^/]+)/(\d+)`
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
	rbeURIRe                      = regexpf("^%s$", rbeURIPatten)
)

// ValidateArtifactID returns a non-nil error if id is invalid.
func ValidateArtifactID(id string) error {
	return validate.SpecifiedWithRe(artifactIDRe, id)
}

// ValidateArtifactIDPrefix returns a non-nil error if prefix is invalid.
func ValidateArtifactIDPrefix(idPrefix string) error {
	return validate.SpecifiedWithRe(artifactIDPrefixRe, idPrefix)
}

// ValidateArtifactName returns a non-nil error if name is invalid.
func ValidateArtifactName(name string) error {
	return validate.SpecifiedWithRe(artifactNameRe, name)
}

// ParseArtifactName extracts the invocation ID, unescaped test id, result ID
// and artifact ID.
// The testID and resultID are empty if this is an invocation-level artifact.
func ParseArtifactName(name string) (invocationID, testID, resultID, artifactID string, err error) {
	if name == "" {
		err = validate.Unspecified()
		return
	}

	unescape := func(escaped string, re *regexp.Regexp) (string, error) {
		unescaped, err := url.PathUnescape(escaped)
		if err != nil {
			return "", errors.Fmt("%q: %w", escaped, err)
		}

		if err := validate.SpecifiedWithRe(re, unescaped); err != nil {
			return "", errors.Fmt("%q: %w", unescaped, err)
		}

		return unescaped, nil
	}

	unescapeTestID := func(escaped string) (string, error) {
		unescaped, err := url.PathUnescape(escaped)
		if err != nil {
			return "", errors.Fmt("%q: %w", escaped, err)
		}

		if err := ValidateTestID(unescaped); err != nil {
			return "", errors.Fmt("%q: %w", unescaped, err)
		}

		return unescaped, nil
	}

	if m := invocationArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		artifactID, err = unescape(m[2], artifactIDRe)
		err = errors.WrapIf(err, "artifact ID")
		return
	}

	if m := testResultArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		if testID, err = unescapeTestID(m[2]); err != nil {
			err = errors.Fmt("test ID: %w", err)
			return
		}
		resultID = m[3]
		artifactID, err = unescape(m[4], artifactIDRe)
		err = errors.WrapIf(err, "artifact ID")
		return
	}

	err = validate.DoesNotMatchReErr(artifactNameRe)
	return
}

// ParseRbeURI parses the RBE URI in the format of
// "bytestream://<HOSTNAME>/projects/<PROJECT_ID>/instances/<INSTANCE_ID>/blobs/<HASH>/<SIZE_BYTES>"
// and then returns the project id, instance id, artifact hash and artifact
// size.
func ParseRbeURI(rbeURI string) (project, instance, hash string, size int64, err error) {
	matches := rbeURIRe.FindStringSubmatch(rbeURI)

	// A successful match will return a slice of 5 strings:
	// matches[0]: The full matched string
	// matches[1]: The Project ID
	// matches[2]: The Instance ID
	// matches[3]: The Hash
	// matches[4]: The Size Bytes (as string)
	if len(matches) != 5 {
		return "", "", "", 0, fmt.Errorf("invalid RBE URI format: does not match 'bytestream://<HOSTNAME>/projects/<PROJECT_ID>/instances/<INSTANCE_ID>/blobs/<HASH>/<SIZE_BYTES>' for URI %q", rbeURI)
	}

	project = matches[1]
	instance = matches[2]
	hash = matches[3]
	size, err = strconv.ParseInt(matches[4], 10, 64)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid size component in RBE URI %q: %w", rbeURI, err)
	}

	return project, instance, hash, size, nil
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
