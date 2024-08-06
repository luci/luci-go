// Copyright 2023 The LUCI Authors.
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
	"strings"

	"go.chromium.org/luci/common/validate"
)

const baselineIDPattern = `[a-z0-9\-_.]{1,100}:[a-zA-Z0-9\-_.\(\) ]{1,128}`

var baselineIDRe = regexpf("^%s$", baselineIDPattern)
var baselineNameRe = regexpf("^projects/%s/baselines/%s", projectPattern, baselineIDPattern)

// ValidateBaselineID returns a non-nil error if the id is invalid.
func ValidateBaselineID(baseline string) error {
	return validate.SpecifiedWithRe(baselineIDRe, baseline)
}

// ParseBaselineName extracts the project and baselineID.
func ParseBaselineName(name string) (project, baselineID string, err error) {
	if name == "" {
		return "", "", validate.Unspecified()
	}

	if m := baselineNameRe.FindStringSubmatch(name); m == nil {
		return "", "", validate.DoesNotMatchReErr(baselineNameRe)
	}

	sp := strings.Split(name, "/")
	return sp[1], sp[3], nil
}
