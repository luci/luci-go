// Copyright 2017 The LUCI Authors.
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

package ensure

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/cipd/client/cipd/common"
	"go.chromium.org/luci/common/errors"
)

var templateParm = regexp.MustCompile(`\${[^}]*}`)

var errSkipTemplate = errors.New("this template should be skipped")

// expandTemplate applies template expansion rules to the package template,
// using the provided os and arch values. If err == errSkipTemplate, that
// means that this template does not apply to this os/arch combination and
// should be skipped.
func expandTemplate(template string, expansionLookup map[string]string) (pkg string, err error) {
	skip := false

	pkg = templateParm.ReplaceAllStringFunc(template, func(parm string) string {
		// ${...}
		contents := parm[2 : len(parm)-1]

		varNameValues := strings.SplitN(contents, "=", 2)
		if len(varNameValues) == 1 {
			// ${varName}
			if value, ok := expansionLookup[varNameValues[0]]; ok {
				return value
			}

			err = errors.Reason("unknown variable in ${%s}", contents).Err()
		}

		// ${varName=value,value}
		ourValue, ok := expansionLookup[varNameValues[0]]
		if !ok {
			err = errors.Reason("unknown variable %q", parm).Err()
			return parm
		}

		for _, val := range strings.Split(varNameValues[1], ",") {
			if val == ourValue {
				return ourValue
			}
		}
		skip = true
		return parm
	})
	if skip {
		err = errSkipTemplate
	}
	if err == nil && strings.ContainsRune(pkg, '$') {
		err = errors.Reason("unable to process some variables in %q", template).Err()
	}
	if err == nil {
		err = common.ValidatePackageName(pkg)
	}
	return
}
