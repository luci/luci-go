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

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// artifactFormatVersion identifies the version of artifact encoding format we're using.
const artifactFormatVersion = 1

const resultIDPattern = `[[:ascii:]]{1,32}`

var testResultNameRe = regexpf("^invocations/(%s)/tests/([^/]+)/results/(%s)$",
	invocationIDPattern, resultIDPattern)
var testIDRe = regexp.MustCompile(`^[[:print:]]{1,256}$`)

// ValidateTestID returns a non-nil error if testID is invalid.
func ValidateTestID(testID string) error {
	if testID == "" {
		return unspecified()
	}
	if !testIDRe.MatchString(testID) {
		return doesNotMatch(testIDRe)
	}
	return nil
}

// ValidateTestResultName returns a non-nil error if name is invalid.
func ValidateTestResultName(name string) error {
	_, _, _, err := ParseTestResultName(name)
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

	if !testIDRe.MatchString(unescapedTestID) {
		err = errors.Annotate(
			doesNotMatch(testIDRe), "test id %q", unescapedTestID).Err()
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
