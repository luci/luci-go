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
	"time"

	"github.com/golang/protobuf/ptypes"
	dpb "github.com/golang/protobuf/ptypes/duration"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// artifactFormatVersion identifies the version of artifact encoding format we're using.
const (
	artifactFormatVersion = 1
	resultIDPattern       = `[[:ascii:]]{1,32}`
	maxLenSummaryHTML     = 4 * 1024
)

var (
	artifactNameRe   = regexp.MustCompile("^[[:word:]]([[:print:]]{0,254}[[:word:]])?$")
	resultIDRe       = regexpf("^%s$", resultIDPattern)
	testIDRe         = regexp.MustCompile(`^[[:print:]]{1,256}$`)
	testResultNameRe = regexpf(
		"^invocations/(%s)/tests/([^/]+)/results/(%s)$", invocationIDPattern,
		resultIDPattern,
	)
)

type checker struct {
	lastCheckedErr *error
}

// isErr returns true if err is nil. False, otherwise.
//
// It also stores err into lastCheckedErr. If err was not nil, it wraps err with
// errors.Annotate before storing it in lastErr.
func (c *checker) isErr(err error, format string, args ...interface{}) bool {
	if err == nil {
		return false
	}
	*c.lastCheckedErr = errors.Annotate(err, format, args...).Err()
	return true
}

// ValidateTestID returns a non-nil error if testID is invalid.
func ValidateTestID(testID string) error {
	return validateWithRe(testIDRe, testID)
}

// ValidateTestResultID returns a non-nil error if resultID is invalid.
func ValidateResultID(resultID string) error {
	return validateWithRe(resultIDRe, resultID)
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

// ValidateStartWithDuration returns a non-nil error if startTime and duration are invalid.
func ValidateStartTimeWithDuration(now time.Time, startTime *tspb.Timestamp, duration *dpb.Duration) error {
	t, err := ptypes.Timestamp(startTime)
	if startTime != nil && err != nil {
		return err
	}

	d, err := ptypes.Duration(duration)
	if duration != nil && err != nil {
		return err
	}

	switch {
	case startTime != nil && now.Before(t):
		return errors.Reason("start_time: cannot be in the future").Err()
	case duration != nil && d < 0:
		return errors.Reason("duration: is < 0").Err()
	case startTime != nil && duration != nil && now.Before(t.Add(d)):
		return errors.Reason("start_time + duration: cannot be in the future").Err()
	}
	return nil
}

// ValidateArtifactName returns a non-nil error if name is invalid.
func ValidateArtifactName(name string) error {
	return validateWithRe(artifactNameRe, name)
}

// ValidateArtifactFetchURL returns a non-nil error if rawurl is invalid.
func ValidateArtifactFetchURL(rawurl string) error {
	switch u, err := url.ParseRequestURI(rawurl); {
	case err != nil:
		return err
	// ParseRequestURI un-capitalizes all the letters of Scheme.
	case u.Scheme != "https":
		return errors.Reason("the URL scheme is not HTTPS").Err()
	case u.Host == "":
		return errors.Reason("missing host").Err()
	}
	return nil
}

// ValidateArtifact returns a non-nil error if art is invalid.
func ValidateArtifact(art *pb.Artifact) (err error) {
	ec := checker{&err}
	switch {
	case art == nil:
		return unspecified()
	case ec.isErr(ValidateArtifactName(art.Name), "name"):
	case ec.isErr(ValidateArtifactFetchURL(art.FetchUrl), "fetch_url"):
		// skip `FetchUrlExpiration`
		// skip `ContentType`
		// skip `Size`
	}
	return err
}

// ValidateArtifacts returns a non-nil error if any element of arts is invalid.
func ValidateArtifacts(arts []*pb.Artifact) error {
	// The name of each Artifact must be unique within the slice.
	names := make(stringset.Set)
	for i, art := range arts {
		if names.Has(art.Name) {
			return errors.Reason("duplicate name %q", art.Name).Err()
		}
		names.Add(art.Name)
		if err := ValidateArtifact(art); err != nil {
			return errors.Annotate(err, "%d", i).Err()
		}
	}
	return nil
}

// ValidateTestResult returns a non-nil error if msg is invalid.
//
// Note that this function validates msg as an RPC request message. Therefore,
// fields with OUTPUT_ONLY tag are not validated.
func ValidateTestResult(now time.Time, msg *pb.TestResult) (err error) {
	ec := checker{&err}
	switch {
	case msg == nil:
		return unspecified()
	// skip `Name`
	case ec.isErr(ValidateTestID(msg.TestId), "test_id"):
	case ec.isErr(ValidateResultID(msg.ResultId), "result_id"):
	case ec.isErr(ValidateVariant(msg.Variant), "variant"):
	// skip `Expected`
	case ec.isErr(ValidateEnum(int32(msg.Status), pb.TestStatus_name), "status"):
	case ec.isErr(ValidateSummaryHTML(msg.SummaryHtml), "summary_html"):
	case ec.isErr(ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration), ""):
	case ec.isErr(ValidateStringPairs(msg.Tags), "tags"):
	case ec.isErr(ValidateArtifacts(msg.InputArtifacts), "input_artifacts"):
	case ec.isErr(ValidateArtifacts(msg.OutputArtifacts), "output_artifacts"):
	}
	return err
}

// ParseTestResultName extracts the invocation ID, unescaped test id, and
// result ID.
func ParseTestResultName(name string) (invID, testID, resultID string, err error) {
	if name == "" {
		err = unspecified()
		return
	}

	m := testResultNameRe.FindStringSubmatch(name)
	if m == nil {
		err = doesNotMatch(testResultNameRe)
		return
	}
	unescapedTestID, err := url.PathUnescape(m[2])
	if err != nil {
		err = errors.Annotate(err, "test id %q", m[2]).Err()
		return
	}

	if ve := validateWithRe(testIDRe, unescapedTestID); ve != nil {
		err = errors.Annotate(ve, "test id %q", unescapedTestID).Err()
		return
	}
	return m[1], unescapedTestID, m[3], nil
}

// TestResultName synthesizes a test result name from its parts.
// Does not validate parts; use ValidateTestResultName.
func TestResultName(invID, testID, resultID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/results/%s",
		invID, url.PathEscape(testID), resultID)
}

// NormalizeTestResult converts inv to the canonical form.
func NormalizeTestResult(tr *pb.TestResult) {
	sortStringPairs(tr.Tags)
	NormalizeArtifactSlice(tr.InputArtifacts)
	NormalizeArtifactSlice(tr.OutputArtifacts)
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

// NormalizeArtifactSlice converts arts to the canonical form.
func NormalizeArtifactSlice(arts []*pb.Artifact) {
	sort.Slice(arts, func(i, j int) bool { return arts[i].Name < arts[j].Name })
}
