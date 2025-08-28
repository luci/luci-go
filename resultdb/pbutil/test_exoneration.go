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
	"fmt"
	"net/url"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"
)

var testExonerationNameRe = regexpf(`^rootInvocations/(%s)/workUnits/(%s)/tests/([^/]+)/exonerations/(.+)$`, rootInvocationIDPattern, workUnitIDPattern)
var legacyTestExonerationNameRe = regexpf(`^invocations/(%s)/tests/([^/]+)/exonerations/(.+)$`, invocationIDPattern)
var exonerationIDRe = regexpf(`^[a-z0-9_\-.:]+$`)

const exonerationIDMaxLength = 150

// TestExonerationName synthesizes a test exoneration name.
// Assumes root invocation, work unit, test and exoneration IDs are valid.
func TestExonerationName(rootInvocationID, workUnitID, testID, exonerationID string) string {
	return fmt.Sprintf("rootInvocations/%s/workUnits/%s/tests/%s/exonerations/%s", rootInvocationID, workUnitID, url.PathEscape(testID), exonerationID)
}

// LegacyTestExonerationName synthesizes a legacy test exoneration name.
// Assumes invocation, test and exoneration IDs are valid.
func LegacyTestExonerationName(invocationID, testID, exonerationID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/exonerations/%s", invocationID, url.PathEscape(testID), exonerationID)
}

// validateExonerationID validates the given exoneration ID is structurally valid.
func ValidateExonerationID(id string) error {
	if len(id) > exonerationIDMaxLength {
		return fmt.Errorf("exoneration ID must be at most %v bytes long", exonerationIDMaxLength)
	}
	return validate.SpecifiedWithRe(exonerationIDRe, id)
}

// TestExonerationNameParts captures the parsed parts of a test exoneration resource name.
type TestExonerationNameParts struct {
	RootInvocationID string
	WorkUnitID       string
	TestID           string
	ExonerationID    string
}

// ParseTestExonerationName extracts root invocation ID, work unit ID,
// test id and exoneration IDs from a test exoneration name.
func ParseTestExonerationName(name string) (TestExonerationNameParts, error) {
	if name == "" {
		return TestExonerationNameParts{}, validate.Unspecified()
	}
	m := testExonerationNameRe.FindStringSubmatch(name)
	if m == nil {
		return TestExonerationNameParts{}, validate.DoesNotMatchReErr(testExonerationNameRe)
	}

	rootInvocationID := m[1]
	if err := ValidateRootInvocationID(rootInvocationID); err != nil {
		return TestExonerationNameParts{}, errors.Fmt("root invocation ID: %w", err)
	}
	workUnitID := m[2]
	if err := ValidateWorkUnitID(workUnitID); err != nil {
		return TestExonerationNameParts{}, errors.Fmt("work unit ID: %w", err)
	}
	testID, err := unescapeAndValidateTestID(m[3])
	if err != nil {
		return TestExonerationNameParts{}, errors.Fmt("test ID: %w", err)
	}
	exonerationID := m[4]
	if err := ValidateExonerationID(exonerationID); err != nil {
		return TestExonerationNameParts{}, errors.Fmt("exoneration ID: %w", err)
	}
	return TestExonerationNameParts{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workUnitID,
		TestID:           testID,
		ExonerationID:    exonerationID,
	}, nil
}

// ParseLegacyTestExonerationName extracts invocation, test id and exoneration IDs
// from a legacy test exoneration name.
func ParseLegacyTestExonerationName(name string) (invocationID, testID, exonerationID string, err error) {
	if name == "" {
		return "", "", "", validate.Unspecified()
	}
	m := legacyTestExonerationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", "", validate.DoesNotMatchReErr(legacyTestExonerationNameRe)
	}
	invocationID = m[1]
	if testID, err = unescapeAndValidateTestID(m[2]); err != nil {
		return "", "", "", err
	}
	exonerationID = m[3]
	if err := ValidateExonerationID(exonerationID); err != nil {
		return "", "", "", errors.Fmt("exoneration ID: %w", err)
	}
	return
}

// ValidateLegacyTestExonerationName returns a non-nil error if the test exoneration
// name is invalid.
func ValidateLegacyTestExonerationName(name string) error {
	_, _, _, err := ParseLegacyTestExonerationName(name)
	return err
}

// IsLegacyTestExonerationName returns whether the given test exoneration name is likely
// a legacy test exoneration name, not a V2 test exoneration name.
// If the name is not valid, this method may return any value.
func IsLegacyTestExonerationName(name string) bool {
	return strings.HasPrefix(name, "invocations/")
}
