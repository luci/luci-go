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
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	resultIDPattern = `[a-z0-9\-_.]{1,32}`
	projectPattern  = `[a-z0-9\-]{1,40}`
	// VariantHashRePattern is the regular expression pattern that matches
	// validly formed Variant Hash.
	variantHashRePattern = `[0-9a-f]{16}`
	// maxLenSummaryHTML is the maximum length of summary HTML.
	maxLenSummaryHTML         = 4 * 1024
	maxLenPrimaryErrorMessage = 1024
	maxLenPrimaryErrorTrace   = 4096
	maxSizeErrors             = 16 * 1024
	maxLenPropertiesSchema    = 256
	// maxClockSkew is the maximum amount of time that clocks could have been out of sync for.
	maxClockSkew = 10 * time.Minute
)

var (
	projectRe              = regexpf("^%s$", projectPattern)
	resultIDRe             = regexpf("^%s$", resultIDPattern)
	variantHashRe          = regexp.MustCompile(`^` + variantHashRePattern + `$`)
	legacyTestResultNameRe = regexpf(
		"^invocations/(%s)/tests/([^/]+)/results/(%s)$", invocationIDPattern,
		resultIDPattern,
	)
	testResultNameRe = regexpf(
		"^rootInvocations/(%s)/workUnits/(%s)/tests/([^/]+)/results/(%s)$", rootInvocationIDPattern, workUnitIDPattern, resultIDPattern,
	)
)

var (
	monorailProjectRe   = regexp.MustCompile(`[a-z0-9]*[a-z]+[a-z0-9]*`)
	monorailComponentRe = regexp.MustCompile(`^[a-zA-Z]([-_]?[a-zA-Z0-9])+(\>[a-zA-Z]([-_]?[a-zA-Z0-9])+)*$`)
	propertiesSchemaRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)
)

// ValidateProject returns a non-nil error if project is invalid.
func ValidateProject(project string) error {
	return validate.SpecifiedWithRe(projectRe, project)
}

// ValidateVariantHash returns a non-nil error if variantHash is invalid.
func ValidateVariantHash(variantHash string) error {
	if variantHash == "" {
		return validate.Unspecified()
	}
	if !variantHashRe.MatchString(variantHash) {
		return errors.Fmt("variant hash %s must match %s", variantHash, variantHashRe)
	}
	return nil
}

// ValidateToScheme represents a callback function that validates a given base test identifier
// matches a configured test scheme.
type ValidateToScheme func(id BaseTestIdentifier) error

// ValidateTestResult validates the given test result is suitable for upload.
//
// For full valdiation, supply a validateToScheme function that validates the test ID
// matches configured scheme. Otherwise, leave it nil.
func ValidateTestResult(now time.Time, validateToScheme ValidateToScheme, tr *pb.TestResult) error {
	if tr == nil {
		return validate.Unspecified()
	}
	if tr.TestIdStructured == nil && tr.TestId != "" {
		// For backwards compatibility, we still accept legacy uploaders setting
		// the test_id and variant fields (even though they are officially OUTPUT_ONLY now).
		testID, err := ParseAndValidateTestID(tr.TestId)
		if err != nil {
			return errors.Fmt("test_id: %w", err)
		}
		// Some legacy clients may set no variant to denote the empty variant.
		if tr.Variant != nil {
			if err := ValidateVariant(tr.Variant); err != nil {
				return errors.Fmt("variant: %w", err)
			}
		}
		if validateToScheme != nil {
			// Validate the test identifier meets the requirements of the scheme.
			// This is enforced only at upload time.
			if err := validateToScheme(testID); err != nil {
				return errors.Fmt("test_id: %w", err)
			}
		}
	} else {
		// Not a legacy uploader.
		// The TestId and Variant fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestIdStructured field.

		if err := ValidateStructuredTestIdentifierForStorage(tr.TestIdStructured); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
		if validateToScheme != nil {
			// Validate the test identifier meets the requirements of the scheme.
			// This is enforced only at upload time.
			if err := validateToScheme(ExtractBaseTestIdentifier(tr.TestIdStructured)); err != nil {
				return errors.Fmt("test_id_structured: %w", err)
			}
		}
	}

	if err := ValidateResultID(tr.ResultId); err != nil {
		return errors.Fmt("result_id: %w", err)
	}
	// Validate StatusV2 if it is specified, or if the status v1 field is unspecified
	// (to trigger "status_v2: must be specified" error).
	if tr.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED || tr.Status == pb.TestStatus_STATUS_UNSPECIFIED {
		if err := ValidateTestResultStatusV2(tr.StatusV2); err != nil {
			return errors.Fmt("status_v2: %w", err)
		}
		// Do not allow status v2 and v1 to be set simultaneously to avoid ambiguities about
		// precedence.
		if tr.Status != pb.TestStatus_STATUS_UNSPECIFIED {
			return errors.New("status: must not specify at same time as status_v2; specify status_v2 only")
		}
		if tr.Expected {
			return errors.New("expected: must not specify at same time as status_v2; specify status_v2 only")
		}

		// Require failure reason to be set for failed results.
		if tr.StatusV2 == pb.TestResult_FAILED {
			if tr.FailureReason == nil {
				return errors.New("failure_reason: must be set when status_v2 is FAILED")
			}
		} else {
			if tr.FailureReason != nil {
				return errors.New("failure_reason: must not be set when status_v2 is not FAILED")
			}
		}
		// Require skipped reason to be set for skipped results.
		if tr.StatusV2 == pb.TestResult_SKIPPED {
			if tr.SkippedReason == nil {
				return errors.New("skipped_reason: must be set when status_v2 is SKIPPED")
			}
		} else {
			if tr.SkippedReason != nil {
				return errors.New("skipped_reason: must not be set when status_v2 is not SKIPPED")
			}
		}
		// Deprecate use of SkipReason enum for old uploads.
		if tr.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
			return errors.New("skip_reason: must not be set in conjuction with status_v2; set skipped_reason.kind instead")
		}
	}
	if tr.Status != pb.TestStatus_STATUS_UNSPECIFIED {
		if err := ValidateTestResultStatus(tr.Status); err != nil {
			return errors.Fmt("status: %w", err)
		}
		// If using legacy status, the failure reason kind should not be set as it
		// could have a conflicting value.
		if tr.FailureReason.GetKind() != pb.FailureReason_KIND_UNSPECIFIED {
			return errors.New("failure_reason: kind: please migrate to using status_v2 if you want to set this field")
		}
		if tr.SkippedReason != nil {
			return errors.New("skipped_reason: please migrate to using status_v2 if you want to set this field")
		}
		if tr.FrameworkExtensions != nil {
			return errors.New("framework_extensions: please migrate to using status_v2 if you want to set this field")
		}
		if err := ValidateTestResultSkipReason(tr.Status, tr.SkipReason); err != nil {
			return errors.Fmt("skip_reason: %w", err)
		}
	}
	if err := ValidateSummaryHTML(tr.SummaryHtml); err != nil {
		return errors.Fmt("summary_html: %w", err)
	}
	if err := ValidateStartTimeWithDuration(now, tr.StartTime, tr.Duration); err != nil {
		return err
	}
	if err := ValidateStringPairs(tr.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}
	if tr.TestMetadata != nil {
		if err := ValidateTestMetadata(tr.TestMetadata); err != nil {
			return errors.Fmt("test_metadata: %w", err)
		}
	}
	if tr.FailureReason != nil {
		if err := ValidateFailureReason(tr.FailureReason, tr.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED); err != nil {
			return errors.Fmt("failure_reason: %w", err)
		}
	}
	if tr.Properties != nil {
		if err := ValidateTestResultProperties(tr.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}
	if tr.SkippedReason != nil {
		if err := ValidateSkippedReason(tr.SkippedReason); err != nil {
			return errors.Fmt("skipped_reason: %w", err)
		}
	}
	if tr.FrameworkExtensions != nil {
		if err := ValidateFrameworkExtensions(tr.FrameworkExtensions, tr.StatusV2); err != nil {
			return errors.Fmt("framework_extensions: %w", err)
		}
	}
	return nil
}

// ValidateResultID returns a non-nil error if resultID is invalid.
func ValidateResultID(resultID string) error {
	return validate.SpecifiedWithRe(resultIDRe, resultID)
}

// ValidateLegacyTestResultName returns a non-nil error if name is an
// invalid legacy test result name.
func ValidateLegacyTestResultName(name string) error {
	_, _, _, err := ParseLegacyTestResultName(name)
	return err
}

// ValidateTestResultName returns a non-nil error if name is invalid
// V2 test result name.
func ValidateTestResultName(name string) error {
	_, err := ParseTestResultName(name)
	return err
}

// ValidateSummaryHTML returns a non-nil error if summary is invalid.
func ValidateSummaryHTML(summary string) error {
	if len(summary) > maxLenSummaryHTML {
		return errors.Fmt("exceeds the maximum size of %d bytes", maxLenSummaryHTML)
	}
	return nil
}

// ValidateStartTimeWithDuration returns a non-nil error if startTime and duration are invalid.
func ValidateStartTimeWithDuration(now time.Time, startTime *timestamppb.Timestamp, duration *durationpb.Duration) error {
	if err := startTime.CheckValid(); err != nil && startTime != nil {
		return err
	}
	t := startTime.AsTime()

	d, err := ptypes.Duration(duration)
	if duration != nil && err != nil {
		return err
	}

	switch {
	case startTime != nil && now.Add(maxClockSkew).Before(t):
		return errors.Fmt("start_time: cannot be > (now + %s), but was +%s", maxClockSkew, t.Sub(now))
	case duration != nil && d < 0:
		return errors.New("duration: is < 0")
	case startTime != nil && duration != nil && now.Add(maxClockSkew).Before(t.Add(d)):
		return errors.Fmt("start_time + duration: cannot be > (now + %s), but was +%s", maxClockSkew, t.Add(d).Sub(now))
	}
	return nil
}

// ValidateTestResultStatus returns a non-nil error if s is invalid for a test result.
func ValidateTestResultStatus(s pb.TestStatus) error {
	if err := ValidateEnum(int32(s), pb.TestStatus_name); err != nil {
		return err
	}
	if s == pb.TestStatus_STATUS_UNSPECIFIED {
		return errors.Fmt("cannot be %s", pb.TestStatus_STATUS_UNSPECIFIED)
	}
	return nil
}

func ValidateTestResultStatusV2(s pb.TestResult_Status) error {
	if err := ValidateEnum(int32(s), pb.TestResult_Status_name); err != nil {
		return err
	}
	if s == pb.TestResult_STATUS_UNSPECIFIED {
		return errors.Fmt("cannot be %s", pb.TestResult_STATUS_UNSPECIFIED)
	}
	return nil
}

// ValidateTestMetadata returns a non-nil error if tmd is invalid.
func ValidateTestMetadata(tmd *pb.TestMetadata) error {
	if tmd.BugComponent != nil {
		if err := ValidateBugComponent(tmd.BugComponent); err != nil {
			return errors.Fmt("bug_component: %w", err)
		}
	}
	if tmd.Location != nil {
		if err := ValidateTestLocation(tmd.Location); err != nil {
			return errors.Fmt("location: %w", err)
		}
	}
	if tmd.PropertiesSchema != "" {
		return errors.Fmt("properties_schema: may not be set")
	}
	if tmd.Properties != nil {
		if err := ValidateTestMetadataProperties(tmd.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}
	if tmd.PreviousTestId != "" {
		if err := ValidateTestID(tmd.PreviousTestId); err != nil {
			return errors.Fmt("previous_test_id: %w", err)
		}
	}
	return nil
}

// ValidatePropertiesSchema returns a non-nil error if properties schema is invalid.
func ValidatePropertiesSchema(propertiesSchema string) error {
	if len(propertiesSchema) > maxLenPropertiesSchema {
		return errors.Fmt("exceeds the maximum size of %d bytes", maxLenPropertiesSchema)
	}
	return validate.SpecifiedWithRe(propertiesSchemaRe, propertiesSchema)
}

// ValidateBugComponent returns a non-nil error if bug component is invalid.
func ValidateBugComponent(bugComponent *pb.BugComponent) error {
	if bugComponent.System == nil {
		return errors.New("bug system is required for bug components")
	}

	switch system := bugComponent.System.(type) {
	case *pb.BugComponent_Monorail:
		{
			// Validate project length is not greater than 63 bytes.
			if len(system.Monorail.Project) > 63 ||
				!monorailProjectRe.MatchString(system.Monorail.Project) {
				return errors.New("monorail.project: is invalid")
			}
			// Validate component length is not greater than 600 bytes.
			if len(system.Monorail.Value) > 600 ||
				!monorailComponentRe.MatchString(system.Monorail.Value) {
				return errors.New("monorail.value: is invalid")
			}
		}
	case *pb.BugComponent_IssueTracker:
		{
			if system.IssueTracker.ComponentId <= 0 {
				return errors.New("issue_tracker.component_id: is invalid")
			}
		}
	default:
		{
			return errors.New("unsupported bug component system")
		}
	}
	return nil
}

// ValidateTestLocation returns a non-nil error if loc is invalid.
func ValidateTestLocation(loc *pb.TestLocation) error {
	switch {
	case loc.GetRepo() == "":
		return errors.New("repo: required")
	case strings.HasSuffix(loc.Repo, ".git"):
		return errors.New("repo: must not end with .git")
	case loc.FileName == "":
		return errors.New("file_name: unspecified")
	case loc.Line < 0:
		return errors.New("line: must not be negative")
	}

	if err := validateFileName(loc.FileName); err != nil {
		return errors.Fmt("file_name: %w", err)
	}
	return nil
}

// ValidateFilePath returns a non-nil error if path is invalid.
func ValidateFilePath(path string) error {
	switch {
	case !strings.HasPrefix(path, "//"):
		return errors.New("doesn't start with //")
	case strings.Contains(path, "\\"):
		return errors.New("has \\")
	case len(path) > 512:
		return errors.New("length exceeds 512")
	}
	return nil
}

func validateFileName(name string) error {
	if strings.HasSuffix(name, "/") {
		return errors.New("ends with /")
	}
	return ValidateFilePath(name)
}

// ValidateFailureReason returns a non-nil error if fr is invalid.
// useStrictValidation should be set if status_v2 is set, this enforces a stricter
// validation of failure reasons.
func ValidateFailureReason(fr *pb.FailureReason, useStrictValidation bool) error {
	if fr == nil {
		return errors.New("unspecified")
	}

	if useStrictValidation {
		if fr.Kind == pb.FailureReason_KIND_UNSPECIFIED {
			return errors.New("kind: unspecified")
		}
	}
	if fr.Kind != pb.FailureReason_KIND_UNSPECIFIED {
		if err := ValidateFailureReasonKind(fr.Kind); err != nil {
			return errors.Fmt("kind: %w", err)
		}
	}

	if useStrictValidation {
		// Clients should have migrated to use the errors collection.
		// The error populated at errors[0] should auto-propagate to primary_error_message.
		if fr.PrimaryErrorMessage != "" {
			return errors.New("primary_error_message: must not be set when status_v2 is set; set errors instead")
		}
	} else {
		if len(fr.PrimaryErrorMessage) > maxLenPrimaryErrorMessage {
			return errors.Fmt("primary_error_message: exceeds the maximum "+
				"size of %d bytes", maxLenPrimaryErrorMessage)
		}
	}

	// Validates the error list:
	// 1. Check if the first error message matches primary_error_message, if supplied
	// 2. Check if any error message exceeds the maximum size
	// 3. Check if the total size of the error list exceeds the maximum size
	totalErrorLen := 0
	for i, e := range fr.Errors {
		if i == 0 && e.Message != fr.PrimaryErrorMessage && fr.PrimaryErrorMessage != "" {
			return errors.New("errors[0]: message: must match primary_error_message, if set")
		}

		if err := ValidateFailureReasonError(e, useStrictValidation); err != nil {
			return errors.Fmt("errors[%d]: %w", i, err)
		}

		totalErrorLen += proto.Size(e)
	}
	if totalErrorLen > maxSizeErrors {
		return errors.Fmt("errors: exceeds the maximum total size of %d "+
			"bytes", maxSizeErrors)
	}

	if fr.TruncatedErrorsCount < 0 {
		return errors.New("truncated_errors_count: must be non-negative")
	}
	return nil
}

// ValidateFailureReasonKind returns a non-nil error if k is invalid.
func ValidateFailureReasonKind(k pb.FailureReason_Kind) error {
	if err := ValidateEnum(int32(k), pb.FailureReason_Kind_name); err != nil {
		return err
	}
	if k == pb.FailureReason_KIND_UNSPECIFIED {
		return errors.Fmt("cannot be %s", pb.FailureReason_KIND_UNSPECIFIED)
	}
	return nil
}

// ValidateFailureReasonError returns a non-nil error if e is invalid.
func ValidateFailureReasonError(e *pb.FailureReason_Error, useStrictValidation bool) error {
	if len(e.Message) > maxLenPrimaryErrorMessage {
		return errors.Fmt("message: exceeds the maximum size of %d bytes",
			maxLenPrimaryErrorMessage)
	}
	if len(e.Trace) > maxLenPrimaryErrorTrace {
		return errors.Fmt("trace: exceeds the maximum size of %d bytes",
			maxLenPrimaryErrorTrace)
	}
	if useStrictValidation {
		if e.Message == "" {
			return errors.New("message: unspecified")
		}
		// We try not to be too pedantic about failure reasons as they often
		// come from raw logs where programs dump random bytes. So we do not enforce
		// 100% printability or Normalisation Form C.
		// However, we do require it to be valid UTF-8 as this is a requirement
		// of all strings passed by proto.
		if !utf8.ValidString(e.Message) {
			return errors.New("message: is not valid UTF-8")
		}
	}
	if !utf8.ValidString(e.Trace) {
		return errors.New("trace: is not valid UTF-8")
	}
	return nil
}

// ValidateSkippedReason returns a non-nil error if sr is invalid.
func ValidateSkippedReason(sr *pb.SkippedReason) error {
	if sr == nil {
		return errors.New("unspecified")
	}
	if err := ValidateSkippedReasonKind(sr.Kind); err != nil {
		return errors.Fmt("kind: %w", err)
	}
	if err := ValidateUTF8Printable(sr.ReasonMessage, 1024, ValidationModeLoose); err != nil {
		return errors.Fmt("reason_message: %w", err)
	}
	if (sr.Kind == pb.SkippedReason_OTHER || sr.Kind == pb.SkippedReason_DEMOTED) && sr.ReasonMessage == "" {
		return errors.Fmt("reason_message: must be set when skipped reason kind is %s", sr.Kind)
	}
	return nil
}

// ValidateSkippedReasonKind returns a non-nil error if k is invalid.
func ValidateSkippedReasonKind(k pb.SkippedReason_Kind) error {
	if err := ValidateEnum(int32(k), pb.SkippedReason_Kind_name); err != nil {
		return err
	}
	if k == pb.SkippedReason_KIND_UNSPECIFIED {
		return errors.Fmt("cannot be %s", pb.SkippedReason_KIND_UNSPECIFIED)
	}
	return nil
}

// ValidateFrameworkExtensions returns a non-nil error if fe is invalid.
func ValidateFrameworkExtensions(fe *pb.FrameworkExtensions, statusV2 pb.TestResult_Status) error {
	if fe == nil {
		return errors.New("unspecified")
	}
	if fe.WebTest != nil {
		if err := ValidateWebTest(fe.WebTest, statusV2); err != nil {
			return errors.Fmt("web_test: %w", err)
		}
	}
	return nil
}

// ValidateWebTest returns a non-nil error if wt is invalid.
func ValidateWebTest(wt *pb.WebTest, statusV2 pb.TestResult_Status) error {
	if wt == nil {
		return errors.New("unspecified")
	}
	if err := ValidateWebTestStatus(wt.Status); err != nil {
		return errors.Fmt("status: %w", err)
	}
	// Besides semantic consistency between web test statuses and status_v2,
	// this constraint exists to ensure ResultDB Indexes on unexpected results
	// can be used to find failed, flaky, execution errored and precluded verdicts.
	if wt.IsExpected {
		if statusV2 == pb.TestResult_FAILED || statusV2 == pb.TestResult_PRECLUDED || statusV2 == pb.TestResult_EXECUTION_ERRORED {
			return errors.Fmt("is_expected: a result with a top-level status_v2 of %s must be marked unexpected", statusV2.String())
		}
	} else {
		if statusV2 == pb.TestResult_PASSED || statusV2 == pb.TestResult_SKIPPED {
			return errors.Fmt("is_expected: a result with a top-level status_v2 of %s must be marked expected", statusV2.String())
		}
	}

	if wt.Status == pb.WebTest_SKIP {
		if statusV2 == pb.TestResult_FAILED || statusV2 == pb.TestResult_PASSED {
			return errors.Fmt("status: a result with a top-level status_v2 of %s must not be marked a web test skip", statusV2.String())
		}
	} else {
		if statusV2 == pb.TestResult_PRECLUDED || statusV2 == pb.TestResult_EXECUTION_ERRORED || statusV2 == pb.TestResult_SKIPPED {
			return errors.Fmt("status: a result with a top-level status_v2 of %s may not be used in conjunction with with a web test fail, pass, crash or timeout", statusV2.String())
		}
	}

	return nil
}

// ValidateWebTestStatus returns a non-nil error if s is invalid.
func ValidateWebTestStatus(s pb.WebTest_Status) error {
	if err := ValidateEnum(int32(s), pb.WebTest_Status_name); err != nil {
		return err
	}
	if s == pb.WebTest_STATUS_UNSPECIFIED {
		return errors.Fmt("cannot be %s", pb.WebTest_STATUS_UNSPECIFIED)
	}
	return nil
}

// ValidateTestResultSkipReason returns a non-nil error if reason is invalid for a test result with the given status.
func ValidateTestResultSkipReason(status pb.TestStatus, reason pb.SkipReason) error {
	if err := ValidateEnum(int32(reason), pb.SkipReason_name); err != nil {
		return err
	}
	if reason != pb.SkipReason_SKIP_REASON_UNSPECIFIED && status != pb.TestStatus_SKIP {
		return errors.New("value must be zero (UNSPECIFIED) when status is not SKIP")
	}
	return nil
}

// ParseLegacyTestResultName extracts the invocation ID, unescaped test id, and
// result ID.
func ParseLegacyTestResultName(name string) (invID, testID, resultID string, err error) {
	if name == "" {
		err = validate.Unspecified()
		return
	}

	m := legacyTestResultNameRe.FindStringSubmatch(name)
	if m == nil {
		err = validate.DoesNotMatchReErr(legacyTestResultNameRe)
		return
	}
	unescapedTestID, err := url.PathUnescape(m[2])
	if err != nil {
		err = errors.Fmt("test id %q: %w", m[2], err)
		return
	}

	if ve := ValidateTestID(unescapedTestID); ve != nil {
		err = errors.Fmt("test id %q: %w", unescapedTestID, ve)
		return
	}
	return m[1], unescapedTestID, m[3], nil
}

type TestResultNameParts struct {
	RootInvocationID string
	WorkUnitID       string
	TestID           string
	ResultID         string
}

// ParseTestResultName parses a V2 test result name into its components.
func ParseTestResultName(name string) (TestResultNameParts, error) {
	if name == "" {
		return TestResultNameParts{}, validate.Unspecified()
	}

	m := testResultNameRe.FindStringSubmatch(name)
	if m == nil {
		return TestResultNameParts{}, validate.DoesNotMatchReErr(testResultNameRe)
	}
	unescapedTestID, err := url.PathUnescape(m[3])
	if err != nil {
		return TestResultNameParts{}, errors.Fmt("test id %q: %w", m[3], err)
	}

	if err := ValidateTestID(unescapedTestID); err != nil {
		return TestResultNameParts{}, errors.Fmt("test id %q: %w", unescapedTestID, err)
	}

	return TestResultNameParts{
		RootInvocationID: m[1],
		WorkUnitID:       m[2],
		TestID:           unescapedTestID,
		ResultID:         m[4],
	}, nil
}

// IsLegacyTestResultName returns whether the given test result name is likely
// a legacy test result name, not a V2 test result name.
// If the name is not valid, this method may return any value.
func IsLegacyTestResultName(name string) bool {
	return strings.HasPrefix(name, "invocations/")
}

// LegacyTestResultName synthesizes a legacy test result name from its parts.
// Does not validate parts; use ValidateTestResultName.
func LegacyTestResultName(invID, testID, resultID string) string {
	return fmt.Sprintf(
		"invocations/%s/tests/%s/results/%s", invID, url.PathEscape(testID), resultID)
}

// TestResultName synthesizes a test result name from its parts.
func TestResultName(rootInvID, workUnitID, testID, resultID string) string {
	return fmt.Sprintf("rootInvocations/%s/workUnits/%s/tests/%s/results/%s", rootInvID, workUnitID, url.PathEscape(testID), resultID)
}

// NormalizeTestResult converts inv to the canonical form.
func NormalizeTestResult(tr *pb.TestResult) {
	SortStringPairs(tr.Tags)
}

// NormalizeTestResultSlice converts trs to the canonical form.
func NormalizeTestResultSlice(trs []*pb.TestResult) {
	for _, tr := range trs {
		NormalizeTestResult(tr)
	}
	sort.Slice(trs, func(i, j int) bool {
		a := trs[i]
		b := trs[j]
		if a.TestId != b.TestId {
			return a.TestId < b.TestId
		}
		return a.Name < b.Name
	})
}
