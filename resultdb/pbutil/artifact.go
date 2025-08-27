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
	"strconv"
	"strings"

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
	// This should be more than enough.
	rbeURIMaxLength = 1024
)

var (
	artifactIDRe                    = regexpf("^%s$", artifactIDPattern)
	artifactIDPrefixRe              = regexpf("^%s$", artifactIDPrefixPattern)
	workUnitArtifactNamePattern     = fmt.Sprintf("rootInvocations/(%s)/workUnits/(%s)/artifacts/(.+)", rootInvocationIDPattern, workUnitIDPattern)
	testResultArtifactNamePattern   = fmt.Sprintf("rootInvocations/(%s)/workUnits/(%s)/tests/([^/]+)/results/(%s)/artifacts/(.+)", rootInvocationIDPattern, workUnitIDPattern, resultIDPattern)
	workUnitArtifactNamePatternRe   = regexpf("^%s$", workUnitArtifactNamePattern)
	testResultArtifactNamePatternRe = regexpf("^%s$", testResultArtifactNamePattern)
	artifactNameRe                  = regexpf("^%s|%s$", testResultArtifactNamePattern, workUnitArtifactNamePattern)

	legacyInvocationArtifactNamePattern = fmt.Sprintf("invocations/(%s)/artifacts/(.+)", invocationIDPattern)
	legacyTestResultArtifactNamePattern = fmt.Sprintf("invocations/(%s)/tests/([^/]+)/results/(%s)/artifacts/(.+)", invocationIDPattern, resultIDPattern)
	legacyInvocationArtifactNameRe      = regexpf("^%s$", legacyInvocationArtifactNamePattern)
	legacyTestResultArtifactNameRe      = regexpf("^%s$", legacyTestResultArtifactNamePattern)
	legacyArtifactNameRe                = regexpf("^%s|%s$", legacyTestResultArtifactNamePattern, legacyInvocationArtifactNamePattern)
	textArtifactContentTypeRe           = regexpf("^text/*")
	rbeURIRe                            = regexpf("^%s$", rbeURIPatten)
)

// ValidateArtifactID returns a non-nil error if id is invalid.
func ValidateArtifactID(id string) error {
	return validate.SpecifiedWithRe(artifactIDRe, id)
}

// ValidateArtifactIDPrefix returns a non-nil error if prefix is invalid.
func ValidateArtifactIDPrefix(idPrefix string) error {
	return validate.SpecifiedWithRe(artifactIDPrefixRe, idPrefix)
}

// ValidateLegacyArtifactName returns a non-nil error if name is invalid.
func ValidateLegacyArtifactName(name string) error {
	return validate.SpecifiedWithRe(legacyArtifactNameRe, name)
}

// ParseArtifactName extracts the root invocation ID, work unit ID, unescaped test id, result ID
// and artifact ID from an artifact name.
// The testID and resultID are empty if this is a work unit-level artifact.
func ParseArtifactName(name string) (rootInvocationID, workUnitID, testID, resultID, artifactID string, err error) {
	if name == "" {
		err = validate.Unspecified()
		return
	}

	if m := workUnitArtifactNamePatternRe.FindStringSubmatch(name); m != nil {
		rootInvocationID = m[1]
		workUnitID = m[2]
		artifactID, err = unescapeAndValidateArtifactID(m[3])
		if err != nil {
			err = errors.Fmt("artifact ID: %w", err)
		}
		return
	}

	if m := testResultArtifactNamePatternRe.FindStringSubmatch(name); m != nil {
		rootInvocationID = m[1]
		workUnitID = m[2]
		testID, err = unescapeAndValidateTestID(m[3])
		if err != nil {
			err = errors.Fmt("test ID: %w", err)
			return
		}
		resultID = m[4]
		artifactID, err = unescapeAndValidateArtifactID(m[5])
		if err != nil {
			err = errors.Fmt("artifact ID: %w", err)
		}
		return
	}

	err = validate.DoesNotMatchReErr(artifactNameRe)
	return
}

// ParseLegacyArtifactName extracts the invocation ID, unescaped test id, result ID
// and artifact ID from a legacy artifact name.
// The testID and resultID are empty if this is an invocation-level artifact.
func ParseLegacyArtifactName(name string) (invocationID, testID, resultID, artifactID string, err error) {
	if name == "" {
		err = validate.Unspecified()
		return
	}

	if m := legacyInvocationArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		artifactID, err = unescapeAndValidateArtifactID(m[2])
		if err != nil {
			err = errors.Fmt("artifact ID: %w", err)
		}
		return
	}

	if m := legacyTestResultArtifactNameRe.FindStringSubmatch(name); m != nil {
		invocationID = m[1]
		testID, err = unescapeAndValidateTestID(m[2])
		if err != nil {
			err = errors.Fmt("test ID: %w", err)
			return
		}
		resultID = m[3]
		artifactID, err = unescapeAndValidateArtifactID(m[4])
		if err != nil {
			err = errors.Fmt("artifact ID: %w", err)
		}
		return
	}

	err = validate.DoesNotMatchReErr(legacyArtifactNameRe)
	return
}

func unescapeAndValidateArtifactID(escaped string) (string, error) {
	unescaped, err := url.PathUnescape(escaped)
	if err != nil {
		return "", errors.Fmt("%q: %w", escaped, err)
	}

	if err := ValidateArtifactID(unescaped); err != nil {
		return "", errors.Fmt("%q: %w", unescaped, err)
	}

	return unescaped, nil
}

// ParseRbeURI parses the RBE URI in the format of
// "bytestream://<HOSTNAME>/projects/<PROJECT_ID>/instances/<INSTANCE_ID>/blobs/<HASH>/<SIZE_BYTES>"
// and then returns the project id, instance id, artifact hash and artifact
// size.
func ParseRbeURI(rbeURI string) (project, instance, hash string, size int64, err error) {
	if len(rbeURI) > rbeURIMaxLength {
		return "", "", "", 0, errors.Fmt("too long; got %d bytes, want max %d", len(rbeURI), rbeURIMaxLength)
	}
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

// RbeInstancePath generates the full RBE instance path in the format of
// "projects/<project_name>/instances/<instance_name>".
func RbeInstancePath(project, instance string) string {
	return fmt.Sprintf("projects/%s/instances/%s", project, instance)
}

// IsLegacyArtifactName returns whether the given artifact name is likely
// a legacy artifact name, not a V2 artifact name.
// If the name is not valid, this method may return any value.
func IsLegacyArtifactName(name string) bool {
	return strings.HasPrefix(name, "invocations/")
}

// WorkUnitArtifactName synthesizes a name of a work unit-level artifact.
// Does not validate IDs, use ValidateRootInvocationID, ValidateWorkUnitID and ValidateArtifactID.
func WorkUnitArtifactName(rootInvID, workUnitID, artifactID string) string {
	return fmt.Sprintf("rootInvocations/%s/workUnits/%s/artifacts/%s", rootInvID, workUnitID, url.PathEscape(artifactID))
}

// TestResultArtifactName synthesizes a name of a test-result-level artifact.
// Does not validate IDs, use ValidateRootInvocationID, ValidateWorkUnitID, ValidateTestID,
// ValidateResultID and ValidateArtifactID.
func TestResultArtifactName(rootInvID, workUnitID, testID, resultID, artifactID string) string {
	return fmt.Sprintf("rootInvocations/%s/workUnits/%s/tests/%s/results/%s/artifacts/%s", rootInvID, workUnitID, url.PathEscape(testID), resultID, url.PathEscape(artifactID))
}

// LegacyInvocationArtifactName synthesizes a name of an invocation-level artifact.
// Does not validate IDs, use ValidateInvocationID and ValidateArtifactID.
func LegacyInvocationArtifactName(invocationID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/artifacts/%s", invocationID, url.PathEscape(artifactID))
}

// LegacyTestResultArtifactName synthesizes a name of an test-result-level artifact.
// Does not validate IDs, use ValidateInvocationID, ValidateTestID,
// ValidateResultID and ValidateArtifactID.
func LegacyTestResultArtifactName(invocationID, testID, resultID, artifactID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/results/%s/artifacts/%s", invocationID, url.PathEscape(testID), resultID, url.PathEscape(artifactID))
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
