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
	"unicode"
	"unicode/utf8"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/text/unicode/norm"
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
	variantHashRePattern      = `[0-9a-f]{16}`
	maxLenSummaryHTML         = 4 * 1024
	maxLenPrimaryErrorMessage = 1024
	maxSizeErrors             = 3*1024 + 100
	maxLenPropertiesSchema    = 256
	// clockSkew is the maxmium amount of time that clocks could have been out of sync for.
	clockSkew = 10 * time.Minute
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

type checker struct {
	lastCheckedErr *error
}

// isErr returns true if err is nil. False, otherwise.
//
// It also stores err into lastCheckedErr. If err was not nil, it wraps err with
// errors.Annotate before storing it in lastErr.
func (c *checker) isErr(err error, format string, args ...any) bool {
	if err == nil {
		return false
	}
	*c.lastCheckedErr = errors.Annotate(err, format, args...).Err()
	return true
}

// ValidateProject returns a non-nil error if project is invalid.
func ValidateProject(project string) error {
	return validate.SpecifiedWithRe(projectRe, project)
}

// ValidateTestID returns a non-nil error if testID is invalid.
func ValidateTestID(testID string) error {
	if testID == "" {
		return validate.Unspecified()
	}
	if len(testID) > 512 {
		return errors.Reason("longer than 512 bytes").Err()
	}
	if !utf8.ValidString(testID) {
		return errors.Reason("not a valid utf8 string").Err()
	}
	if !norm.NFC.IsNormalString(testID) {
		return errors.Reason("not in unicode normalized form C").Err()
	}
	for i, rune := range testID {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
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
	case startTime != nil && now.Add(clockSkew).Before(t):
		return errors.Reason("start_time: cannot be > (now + %s), but was +%s", clockSkew, t.Sub(now)).Err()
	case duration != nil && d < 0:
		return errors.Reason("duration: is < 0").Err()
	case startTime != nil && duration != nil && now.Add(clockSkew).Before(t.Add(d)):
		return errors.Reason("start_time + duration: cannot be > (now + %s), but was +%s", clockSkew, t.Add(d).Sub(now)).Err()
	}
	return nil
}

// ValidateTestResult returns a non-nil error if msg is invalid.
func ValidateTestResult(now time.Time, msg *pb.TestResult) (err error) {
	ec := checker{&err}
	switch {
	case msg == nil:
		return validate.Unspecified()
	// skip `Name`
	case ec.isErr(ValidateTestID(msg.TestId), "test_id"):
	case ec.isErr(ValidateResultID(msg.ResultId), "result_id"):
	case ec.isErr(ValidateVariant(msg.Variant), "variant"):
	// skip `Expected`
	case ec.isErr(ValidateTestResultStatus(msg.Status), "status"):
	case ec.isErr(ValidateSummaryHTML(msg.SummaryHtml), "summary_html"):
	case ec.isErr(ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration), ""):
	case ec.isErr(ValidateStringPairs(msg.Tags), "tags"):
	case msg.TestMetadata != nil && ec.isErr(ValidateTestMetadata(msg.TestMetadata), "test_metadata"):
	case msg.FailureReason != nil && ec.isErr(ValidateFailureReason(msg.FailureReason), "failure_reason"):
	case msg.Properties != nil && ec.isErr(ValidateTestResultProperties(msg.Properties), "properties"):
	case ec.isErr(ValidateTestResultSkipReason(msg.Status, msg.SkipReason), "skip_reason"):
	}
	return err
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
func ValidateFailureReason(fr *pb.FailureReason) error {
	if len(fr.PrimaryErrorMessage) > maxLenPrimaryErrorMessage {
		return errors.Reason("primary_error_message: exceeds the maximum "+
			"size of %d bytes", maxLenPrimaryErrorMessage).Err()
	}

	// Validates the error list:
	// 1. Check if the first error message matches primary_error_message
	// 2. Check if any error message exceeds the maximum size
	// 3. Check if the total size of the error list exceeds the maximum size
	totalErrorLen := 0
	for i, error := range fr.Errors {
		if i == 0 && error.Message != fr.PrimaryErrorMessage {
			return errors.Reason(
				"errors[0]: message: must match primary_error_message").Err()
		}

		if err := ValidateError(error); err != nil {
			return errors.Annotate(err, "errors[%d]", i).Err()
		}

		totalErrorLen += proto.Size(error)
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

func ValidateError(error *pb.FailureReason_Error) error {
	if len(error.GetMessage()) > maxLenPrimaryErrorMessage {
		return errors.Reason(
			"message: exceeds the maximum size of %d bytes",
			maxLenPrimaryErrorMessage).Err()
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
