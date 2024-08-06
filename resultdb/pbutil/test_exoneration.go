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

	"go.chromium.org/luci/common/validate"
)

var testExonerationNameRe = regexpf(`^invocations/(%s)/tests/([^/]+)/exonerations/(.+)$`, invocationIDPattern)

// TestExonerationName synthesizes a test exoneration name.
// Assumes invocation and exoneration IDs are valid.
func TestExonerationName(invocationID, testID, exonerationID string) string {
	return fmt.Sprintf("invocations/%s/tests/%s/exonerations/%s", invocationID, url.PathEscape(testID), exonerationID)
}

// ParseTestExonerationName extracts invocation, test id and exoneration IDs
// from the name.
func ParseTestExonerationName(name string) (invocationID, testID, exonerationID string, err error) {
	if name == "" {
		return "", "", "", validate.Unspecified()
	}
	m := testExonerationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", "", validate.DoesNotMatchReErr(testExonerationNameRe)
	}
	invocationID = m[1]
	if testID, err = url.PathUnescape(m[2]); err != nil {
		return "", "", "", err
	}
	exonerationID = m[3]
	return
}

// ValidateTestExonerationName returns a non-nil error if the test exoneration
// name is invalid.
func ValidateTestExonerationName(name string) error {
	_, _, _, err := ParseTestExonerationName(name)
	return err
}
