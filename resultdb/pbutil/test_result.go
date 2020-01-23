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

	"go.chromium.org/luci/common/errors"

	"github.com/golang/protobuf/ptypes"
	dpb "github.com/golang/protobuf/ptypes/duration"
	tpb "github.com/golang/protobuf/ptypes/timestamp"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// artifactFormatVersion identifies the version of artifact encoding format we're using.
const (
	artifactFormatVersion = 1
	resultIDPattern       = `[[:ascii:]]{1,32}`
	maxLenSummaryHtml     = 4 * 1024
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

// ValidateSummaryHtml returns a non-nil error if summary is invalid.
func ValidateSummaryHtml(summary string) error {
	if len(summary) > maxLenSummaryHtml {
		return errors.Reason("exceeds the maximum length of %d", maxLenSummaryHtml).Err()
	}
	return nil
}

// ValidateStartWithDuration returns a non-nil error if st and d are invalid.
func ValidateStartTimeWithDuration(pt *tpb.Timestamp, pd *dpb.Duration) error {
	t, err := ptypes.Timestamp(pt)
	if err != nil {
		return err
	}
	d, err := ptypes.Duration(pd)
	if err != nil {
		return err
	}

	now := time.Now()
	if now.Before(t) {
		return errors.Reason("start_time cannot be future").Err()
	}
	if d < 0 {
		return errors.Reason("duration is < 0").Err()
	}
	if now.Before(t.Add(d)) {
		return errors.Reason("duration is invalid.").Err()
	}
	return nil
}

// ValidateArtifactName returns a non-nil error if name is invalid.
func ValidateArtifactName(name string) error {
	return validateWithRe(artifactNameRe, name)
}

// ValidateArtifactFetchUrl returns a non-nil error if rawurl is invalid.
func ValidateArtifactFetchUrl(rawurl string) error {
	u, err := url.ParseRequestURI(rawurl)
	if err != nil {
		return err
	}
	// ParseRequestURI un-capitalizes all the letters of Scheme.
	if u.Scheme != "https" {
		return errors.Reason("the URL scheme is not HTTPS").Err()
	}
	if u.Host == "" {
		return errors.Reason("missing host").Err()
	}
	return nil
}

// ValidateArtifact returns a non-nil error if art is invalid.
func ValidateArtifact(art *pb.Artifact) (err error) {
	isErr := func(ve error) bool {
		err = ve
		return ve != nil
	}

	switch {
	case isErr(ValidateArtifactName(art.Name)):
	case isErr(ValidateArtifactFetchUrl(art.FetchUrl)):
		// skip `FetchUrlExpiration`
		// skip `ContentType`
		// skip `Size`
		break
	}
	return
}

// ValidateArtifacts returns a non-nil error if any elemnt of arts is invalid.
func ValidateArtifacts(arts []*pb.Artifact) error {
	for _, art := range arts {
		if err := ValidateArtifact(art); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTestResult returns a non-nil error if msg is invalid.
//
// Note that this function validates msg as an RPC request message. Therefore,
// fields with OUTPUT_ONLY tag are not validated.
func ValidateTestResult(msg *pb.TestResult) (err error) {
	isErr := func(ve error) bool {
		err = ve
		return ve != nil
	}

	switch {
	// skip `Name`
	case isErr(ValidateTestID(msg.TestId)):
	case isErr(ValidateResultID(msg.ResultId)):
	case isErr(ValidateVariant(msg.Variant)):
	// skip `Expected`
	case isErr(ValidateEnum(int32(msg.Status), pb.TestStatus_name)):
	case isErr(ValidateSummaryHtml(msg.SummaryHtml)):
	case isErr(ValidateStartTimeWithDuration(msg.StartTime, msg.Duration)):
	case isErr(ValidateStringPairs(msg.Tags)):
	case isErr(ValidateArtifacts(msg.InputArtifacts)):
	case isErr(ValidateArtifacts(msg.OutputArtifacts)):
		// validation error!
		break
	}
	return
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
