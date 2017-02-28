// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ensure

import (
	"regexp"
	"strings"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/common/errors"
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

			err = errors.Reason("unknown variable in ${%(contents)s}").
				D("contents", contents).Err()
		}

		// ${varName=value,value}
		ourValue, ok := expansionLookup[varNameValues[0]]
		if !ok {
			err = errors.Reason("unknown variable %(parm)q").D("parm", parm).Err()
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
		err = errors.Reason("unable to process some variables in %(template)q").
			D("template", template).Err()
	}
	if err == nil {
		err = common.ValidatePackageName(pkg)
	}
	return
}
