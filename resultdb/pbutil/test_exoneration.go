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

	pb "go.chromium.org/luci/resultdb/proto/v1"
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

// ValidateTestExoneration validates the given test exoneration is suitable for upload.
func ValidateTestExoneration(ex *pb.TestExoneration, validateToScheme ValidateToScheme, getLimits TestIDValidationLimitsCallback, strictValidation bool) error {
	if ex == nil {
		return errors.New("unspecified")
	}
	if validateToScheme == nil {
		panic("validateToScheme is required")
	}
	if ex.TestIdStructured == nil && ex.TestId != "" {
		// For backwards compatibility, we still accept legacy uploaders setting
		// the test_id and variant or variant_hash fields (even though they are
		// officially OUTPUT_ONLY now).
		testID, err := ParseAndValidateTestID(ex.TestId, getLimits)
		if err != nil {
			return errors.Fmt("test_id: %w", err)
		}
		// Legacy clients may not set Variant, or use nil to represent
		// the empty variant. As this is ambiguous, we rely on the absence
		// of VariantHash being set to determine if Variant was deliberately
		// set to nil or if the Variant is simply not set.
		//
		// In strict validation: the variant is always required.
		if ex.Variant != nil || strictValidation {
			if err := ValidateVariant(ex.GetVariant()); err != nil {
				return errors.Fmt("variant: %w", err)
			}
		}

		// Some legacy clients do not set variant and only set variant_hash.
		// Some set both. If both are set, check they are consistent.
		hasVariant := len(ex.Variant.GetDef()) != 0 || strictValidation
		hasVariantHash := ex.VariantHash != ""
		if hasVariant && hasVariantHash {
			computedHash := VariantHash(ex.GetVariant())
			if computedHash != ex.VariantHash {
				return errors.New("computed and supplied variant hash don't match")
			}
		}
		if hasVariantHash {
			if err := ValidateVariantHash(ex.VariantHash); err != nil {
				return errors.Fmt("variant_hash: %w", err)
			}
		}

		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateToScheme(testID); err != nil {
			return errors.Fmt("test_id: %w", err)
		}
	} else {
		// Not a legacy uploader.
		// The TestId, Variant, VariantHash fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestIdStructured field.
		// Note that TestIdStructured.ModuleVariantHash is also output only and should
		// also be ignored.

		if err := ValidateStructuredTestIdentifierForStorage(ex.TestIdStructured, getLimits); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}

		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateToScheme(ExtractBaseTestIdentifier(ex.TestIdStructured)); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
	}

	if ex.ExplanationHtml == "" {
		return errors.New("explanation_html: unspecified")
	}
	if ex.Reason == pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
		return errors.New("reason: unspecified")
	}
	return nil
}
