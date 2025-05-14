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
	maxSizeErrors             = 3*1024 + 100
	maxLenPropertiesSchema    = 256
	// maxClockSkew is the maximum amount of time that clocks could have been out of sync for.
	maxClockSkew = 10 * time.Minute
)

var (
	projectRe        = regexpf("^%s$", projectPattern)
	resultIDRe       = regexpf("^%s$", resultIDPattern)
	variantHashRe    = regexp.MustCompile(`^` + variantHashRePattern + `$`)
	testResultNameRe = regexpf(
		"^invocations/(%s)/tests/([^/]+)/results/(%s)$", invocationIDPattern,
		resultIDPattern,
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
		return errors.Reason("variant hash %s must match %s", variantHash, variantHashRe).Err()
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
			return errors.Annotate(err, "test_id").Err()
		}
		// Some legacy clients may set no variant to denote the empty variant.
		if tr.Variant != nil {
			if err := ValidateVariant(tr.Variant); err != nil {
				return errors.Annotate(err, "variant").Err()
			}
		}
		if validateToScheme != nil {
			// Validate the test identifier meets the requirements of the scheme.
			// This is enforced only at upload time.
			if err := validateToScheme(testID); err != nil {
				return errors.Annotate(err, "test_id").Err()
			}
		}
	} else {
		// Not a legacy uploader.
		// The TestId and Variant fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestIdStructured field.

		if err := ValidateStructuredTestIdentifierForStorage(tr.TestIdStructured); err != nil {
			return errors.Annotate(err, "test_id_structured").Err()
		}
		if validateToScheme != nil {
			// Validate the test identifier meets the requirements of the scheme.
			// This is enforced only at upload time.
			if err := validateToScheme(ExtractBaseTestIdentifier(tr.TestIdStructured)); err != nil {
				return errors.Annotate(err, "test_id_structured").Err()
			}
		}
	}

	if err := ValidateResultID(tr.ResultId); err != nil {
		return errors.Annotate(err, "result_id").Err()
	}
	// Validate StatusV2 if it is specified, or if the status v1 field is unspecified
	// (to trigger "status_v2: must be specified" error).
	if tr.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED || tr.Status == pb.TestStatus_STATUS_UNSPECIFIED {
		if err := ValidateTestResultStatusV2(tr.StatusV2); err != nil {
			return errors.Annotate(err, "status_v2").Err()
		}
		// Do not allow status v2 and v1 to be set simultaneously to avoid ambiguities about
		// precedence.
		if tr.Status != pb.TestStatus_STATUS_UNSPECIFIED {
			return errors.Reason("status: must not specify at same time as status_v2; specify status_v2 only").Err()
		}
		if tr.Expected {
			return errors.Reason("expected: must not specify at same time as status_v2; specify status_v2 only").Err()
		}

		// Require failure reason to be set for failed results.
		if tr.StatusV2 == pb.TestResult_FAILED {
			if tr.FailureReason == nil {
				return errors.Reason("failure_reason: must be set when status_v2 is FAILED").Err()
			}
		} else {
			if tr.FailureReason != nil {
				return errors.Reason("failure_reason: must not be set when status_v2 is not FAILED").Err()
			}
		}
		// Require skipped reason to be set for skipped results.
		if tr.StatusV2 == pb.TestResult_SKIPPED {
			if tr.SkippedReason == nil {
				return errors.Reason("skipped_reason: must be set when status_v2 is SKIPPED").Err()
			}
		} else {
			if tr.SkippedReason != nil {
				return errors.Reason("skipped_reason: must not be set when status_v2 is not SKIPPED").Err()
			}
		}
		// Deprecate use of SkipReason enum for old uploads.
		if tr.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
			return errors.Reason("skip_reason: must not be set in conjuction with status_v2; set skipped_reason.kind instead").Err()
		}
	}
	if tr.Status != pb.TestStatus_STATUS_UNSPECIFIED {
		if err := ValidateTestResultStatus(tr.Status); err != nil {
			return errors.Annotate(err, "status").Err()
		}
		// If using legacy status, the failure reason kind should not be set as it
		// could have a conflicting value.
		if tr.FailureReason.GetKind() != pb.FailureReason_KIND_UNSPECIFIED {
			return errors.Reason("failure_reason: kind: please migrate to using status_v2 if you want to set this field").Err()
		}
		if tr.SkippedReason != nil {
			return errors.Reason("skipped_reason: please migrate to using status_v2 if you want to set this field").Err()
		}
		if tr.FrameworkExtensions != nil {
			return errors.Reason("framework_extensions: please migrate to using status_v2 if you want to set this field").Err()
		}
		if err := ValidateTestResultSkipReason(tr.Status, tr.SkipReason); err != nil {
			return errors.Annotate(err, "skip_reason").Err()
		}
	}
	if err := ValidateSummaryHTML(tr.SummaryHtml); err != nil {
		return errors.Annotate(err, "summary_html").Err()
	}
	if err := ValidateStartTimeWithDuration(now, tr.StartTime, tr.Duration); err != nil {
		return err
	}
	if err := ValidateStringPairs(tr.Tags); err != nil {
		return errors.Annotate(err, "tags").Err()
	}
	if tr.TestMetadata != nil {
		if err := ValidateTestMetadata(tr.TestMetadata); err != nil {
			return errors.Annotate(err, "test_metadata").Err()
		}
	}
	if tr.FailureReason != nil {
		if err := ValidateFailureReason(tr.FailureReason, tr.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED); err != nil {
			return errors.Annotate(err, "failure_reason").Err()
		}
	}
	if tr.Properties != nil {
		if err := ValidateTestResultProperties(tr.Properties); err != nil {
			return errors.Annotate(err, "properties").Err()
		}
	}
	if tr.SkippedReason != nil {
		if err := ValidateSkippedReason(tr.SkippedReason); err != nil {
			return errors.Annotate(err, "skipped_reason").Err()
		}
	}
	if tr.FrameworkExtensions != nil {
		if err := ValidateFrameworkExtensions(tr.FrameworkExtensions, tr.StatusV2); err != nil {
			return errors.Annotate(err, "framework_extensions").Err()
		}
	}
	return nil
}

// ValidateResultID returns a non-nil error if resultID is invalid.
func ValidateResultID(resultID string) error {
	return validate.SpecifiedWithRe(resultIDRe, resultID)
}

// ValidateTestResultName returns a non-nil error if name is invalid.
func ValidateTestResultName(name string) error {
	_, _, _, err := ParseTestResultName(name)
	return err
}

// ValidateSummaryHTML returns a non-nil error if summary is invalid.
func ValidateSummaryHTML(summary string) error {
	if len(summary) > maxLenSummaryHTML {
		return errors.Reason("exceeds the maximum size of %d bytes", maxLenSummaryHTML).Err()
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
		return errors.Reason("start_time: cannot be > (now + %s), but was +%s", maxClockSkew, t.Sub(now)).Err()
	case duration != nil && d < 0:
		return errors.Reason("duration: is < 0").Err()
	case startTime != nil && duration != nil && now.Add(maxClockSkew).Before(t.Add(d)):
		return errors.Reason("start_time + duration: cannot be > (now + %s), but was +%s", maxClockSkew, t.Add(d).Sub(now)).Err()
	}
	return nil
}

// ValidateTestResultStatus returns a non-nil error if s is invalid for a test result.
func ValidateTestResultStatus(s pb.TestStatus) error {
	if err := ValidateEnum(int32(s), pb.TestStatus_name); err != nil {
		return err
	}
	if s == pb.TestStatus_STATUS_UNSPECIFIED {
		return errors.Reason("cannot be %s", pb.TestStatus_STATUS_UNSPECIFIED).Err()
	}
	return nil
}

func ValidateTestResultStatusV2(s pb.TestResult_Status) error {
	if err := ValidateEnum(int32(s), pb.TestResult_Status_name); err != nil {
		return err
	}
	if s == pb.TestResult_STATUS_UNSPECIFIED {
		return errors.Reason("cannot be %s", pb.TestResult_STATUS_UNSPECIFIED).Err()
	}
	return nil
}

// ValidateTestMetadata returns a non-nil error if tmd is invalid.
func ValidateTestMetadata(tmd *pb.TestMetadata) error {
	if tmd.BugComponent != nil {
		if err := ValidateBugComponent(tmd.BugComponent); err != nil {
			return errors.Annotate(err, "bug_component").Err()
		}
	}
	if tmd.Location != nil {
		if err := ValidateTestLocation(tmd.Location); err != nil {
			return errors.Annotate(err, "location").Err()
		}
	}
	if tmd.PropertiesSchema != "" {
		if err := ValidatePropertiesSchema(tmd.PropertiesSchema); err != nil {
			return errors.Annotate(err, "properties_schema").Err()
		}
	}
	if tmd.Properties != nil {
		if tmd.PropertiesSchema == "" {
			return errors.New("properties_schema must be specified with non-empty properties")
		}
		if err := ValidateTestMetadataProperties(tmd.Properties); err != nil {
			return errors.Annotate(err, "properties").Err()
		}
	}
	if tmd.PreviousTestId != "" {
		if err := ValidateTestID(tmd.PreviousTestId); err != nil {
			return errors.Annotate(err, "previous_test_id").Err()
		}
	}
	return nil
}

// ValidatePropertiesSchema returns a non-nil error if properties schema is invalid.
func ValidatePropertiesSchema(propertiesSchema string) error {
	if len(propertiesSchema) > maxLenPropertiesSchema {
		return errors.Reason("exceeds the maximum size of %d bytes", maxLenPropertiesSchema).Err()
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
		return errors.Reason("repo: required").Err()
	case strings.HasSuffix(loc.Repo, ".git"):
		return errors.Reason("repo: must not end with .git").Err()
	case loc.FileName == "":
		return errors.Reason("file_name: unspecified").Err()
	case loc.Line < 0:
		return errors.Reason("line: must not be negative").Err()
	}

	if err := validateFileName(loc.FileName); err != nil {
		return errors.Annotate(err, "file_name").Err()
	}
	return nil
}

// ValidateFilePath returns a non-nil error if path is invalid.
func ValidateFilePath(path string) error {
	switch {
	case !strings.HasPrefix(path, "//"):
		return errors.Reason("doesn't start with //").Err()
	case strings.Contains(path, "\\"):
		return errors.Reason("has \\").Err()
	case len(path) > 512:
		return errors.Reason("length exceeds 512").Err()
	}
	return nil
}

func validateFileName(name string) error {
	if strings.HasSuffix(name, "/") {
		return errors.Reason("ends with /").Err()
	}
	return ValidateFilePath(name)
}

// ValidateFailureReason returns a non-nil error if fr is invalid.
// useStrictValidation should be set if status_v2 is set, this enforces a stricter
// validation of failure reasons.
func ValidateFailureReason(fr *pb.FailureReason, useStrictValidation bool) error {
	if fr == nil {
		return errors.Reason("unspecified").Err()
	}

	if useStrictValidation {
		if fr.Kind == pb.FailureReason_KIND_UNSPECIFIED {
			return errors.Reason("kind: unspecified").Err()
		}
	}
	if fr.Kind != pb.FailureReason_KIND_UNSPECIFIED {
		if err := ValidateFailureReasonKind(fr.Kind); err != nil {
			return errors.Annotate(err, "kind").Err()
		}
	}

	if useStrictValidation {
		// Clients should have migrated to use the errors collection.
		// The error populated at errors[0] should auto-propagate to primary_error_message.
		if fr.PrimaryErrorMessage != "" {
			return errors.Reason("primary_error_message: must not be set when status_v2 is set; set errors instead").Err()
		}
	} else {
		if len(fr.PrimaryErrorMessage) > maxLenPrimaryErrorMessage {
			return errors.Reason("primary_error_message: exceeds the maximum "+
				"size of %d bytes", maxLenPrimaryErrorMessage).Err()
		}
	}

	// Validates the error list:
	// 1. Check if the first error message matches primary_error_message, if supplied
	// 2. Check if any error message exceeds the maximum size
	// 3. Check if the total size of the error list exceeds the maximum size
	totalErrorLen := 0
	for i, e := range fr.Errors {
		if i == 0 && e.Message != fr.PrimaryErrorMessage && fr.PrimaryErrorMessage != "" {
			return errors.Reason(
				"errors[0]: message: must match primary_error_message, if set").Err()
		}

		if err := ValidateFailureReasonError(e, useStrictValidation); err != nil {
			return errors.Annotate(err, "errors[%d]", i).Err()
		}

		totalErrorLen += proto.Size(e)
	}
	if totalErrorLen > maxSizeErrors {
		return errors.Reason("errors: exceeds the maximum total size of %d "+
			"bytes", maxSizeErrors).Err()
	}

	if fr.TruncatedErrorsCount < 0 {
		return errors.Reason(
			"truncated_errors_count: must be non-negative").Err()
	}
	return nil
}

// ValidateFailureReasonKind returns a non-nil error if k is invalid.
func ValidateFailureReasonKind(k pb.FailureReason_Kind) error {
	if err := ValidateEnum(int32(k), pb.FailureReason_Kind_name); err != nil {
		return err
	}
	if k == pb.FailureReason_KIND_UNSPECIFIED {
		return errors.Reason("cannot be %s", pb.FailureReason_KIND_UNSPECIFIED).Err()
	}
	return nil
}

// ValidateFailureReasonError returns a non-nil error if e is invalid.
func ValidateFailureReasonError(e *pb.FailureReason_Error, useStrictValidation bool) error {
	if len(e.GetMessage()) > maxLenPrimaryErrorMessage {
		return errors.Reason(
			"message: exceeds the maximum size of %d bytes",
			maxLenPrimaryErrorMessage).Err()
	}
	if useStrictValidation {
		if e.Message == "" {
			return errors.Reason("message: unspecified").Err()
		}
		// We try not to be too pedantic about failure reasons as they often
		// come from raw logs where programs dump random bytes. So we do not enforce
		// 100% printability or Normalisation Form C.
		// However, we do require it to be valid UTF-8 as this is a requirement
		// of all strings passed by proto.
		if !utf8.ValidString(e.Message) {
			return errors.Reason("message: is not valid UTF-8").Err()
		}
	}
	return nil
}

// ValidateSkippedReason returns a non-nil error if sr is invalid.
func ValidateSkippedReason(sr *pb.SkippedReason) error {
	if sr == nil {
		return errors.Reason("unspecified").Err()
	}
	if err := ValidateSkippedReasonKind(sr.Kind); err != nil {
		return errors.Annotate(err, "kind").Err()
	}
	if err := ValidateUTF8Printable(sr.ReasonMessage, 1024, ValidationModeLoose); err != nil {
		return errors.Annotate(err, "reason_message").Err()
	}
	if (sr.Kind == pb.SkippedReason_OTHER || sr.Kind == pb.SkippedReason_DEMOTED) && sr.ReasonMessage == "" {
		return errors.Reason("reason_message: must be set when skipped reason kind is %s", sr.Kind).Err()
	}
	return nil
}

// ValidateSkippedReasonKind returns a non-nil error if k is invalid.
func ValidateSkippedReasonKind(k pb.SkippedReason_Kind) error {
	if err := ValidateEnum(int32(k), pb.SkippedReason_Kind_name); err != nil {
		return err
	}
	if k == pb.SkippedReason_KIND_UNSPECIFIED {
		return errors.Reason("cannot be %s", pb.SkippedReason_KIND_UNSPECIFIED).Err()
	}
	return nil
}

// ValidateFrameworkExtensions returns a non-nil error if fe is invalid.
func ValidateFrameworkExtensions(fe *pb.FrameworkExtensions, statusV2 pb.TestResult_Status) error {
	if fe == nil {
		return errors.Reason("unspecified").Err()
	}
	if fe.WebTest != nil {
		if err := ValidateWebTest(fe.WebTest, statusV2); err != nil {
			return errors.Annotate(err, "web_test").Err()
		}
	}
	return nil
}

// ValidateWebTest returns a non-nil error if wt is invalid.
func ValidateWebTest(wt *pb.WebTest, statusV2 pb.TestResult_Status) error {
	if wt == nil {
		return errors.Reason("unspecified").Err()
	}
	if err := ValidateWebTestStatus(wt.Status); err != nil {
		return errors.Annotate(err, "status").Err()
	}
	// Besides semantic consistency between web test statuses and status_v2,
	// this constraint exists to ensure ResultDB Indexes on unexpected results
	// can be used to find failed, flaky, execution errored and precluded verdicts.
	if wt.IsExpected {
		if statusV2 == pb.TestResult_FAILED || statusV2 == pb.TestResult_PRECLUDED || statusV2 == pb.TestResult_EXECUTION_ERRORED {
			return errors.Reason("is_expected: a result with a top-level status_v2 of %s must be marked unexpected", statusV2.String()).Err()
		}
	} else {
		if statusV2 == pb.TestResult_PASSED || statusV2 == pb.TestResult_SKIPPED {
			return errors.Reason("is_expected: a result with a top-level status_v2 of %s must be marked expected", statusV2.String()).Err()
		}
	}

	if wt.Status == pb.WebTest_SKIP {
		if statusV2 == pb.TestResult_FAILED || statusV2 == pb.TestResult_PASSED {
			return errors.Reason("status: a result with a top-level status_v2 of %s must not be marked a web test skip", statusV2.String()).Err()
		}
	} else {
		if statusV2 == pb.TestResult_PRECLUDED || statusV2 == pb.TestResult_EXECUTION_ERRORED || statusV2 == pb.TestResult_SKIPPED {
			return errors.Reason("status: a result with a top-level status_v2 of %s may not be used in conjunction with with a web test fail, pass, crash or timeout", statusV2.String()).Err()
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
		return errors.Reason("cannot be %s", pb.WebTest_STATUS_UNSPECIFIED).Err()
	}
	return nil
}

// ValidateTestResultSkipReason returns a non-nil error if reason is invalid for a test result with the given status.
func ValidateTestResultSkipReason(status pb.TestStatus, reason pb.SkipReason) error {
	if err := ValidateEnum(int32(reason), pb.SkipReason_name); err != nil {
		return err
	}
	if reason != pb.SkipReason_SKIP_REASON_UNSPECIFIED && status != pb.TestStatus_SKIP {
		return errors.Reason("value must be zero (UNSPECIFIED) when status is not SKIP").Err()
	}
	return nil
}

// ParseTestResultName extracts the invocation ID, unescaped test id, and
// result ID.
func ParseTestResultName(name string) (invID, testID, resultID string, err error) {
	if name == "" {
		err = validate.Unspecified()
		return
	}

	m := testResultNameRe.FindStringSubmatch(name)
	if m == nil {
		err = validate.DoesNotMatchReErr(testResultNameRe)
		return
	}
	unescapedTestID, err := url.PathUnescape(m[2])
	if err != nil {
		err = errors.Annotate(err, "test id %q", m[2]).Err()
		return
	}

	if ve := ValidateTestID(unescapedTestID); ve != nil {
		err = errors.Annotate(ve, "test id %q", unescapedTestID).Err()
		return
	}
	return m[1], unescapedTestID, m[3], nil
}

// TestResultName synthesizes a test result name from its parts.
// Does not validate parts; use ValidateTestResultName.
func TestResultName(invID, testID, resultID string) string {
	return fmt.Sprintf(
		"invocations/%s/tests/%s/results/%s", invID, url.PathEscape(testID), resultID)
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
