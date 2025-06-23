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

// Package stringtemplate implements Python string.Template-like substitution.
package stringtemplate

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

const idPattern = "[_a-z][_a-z0-9]*"

// We're looking for:
//
// Submatch indices:
// [0:1] Full match
// [2:3] $$ (escaped)
// [4:5] $key (without braces)
// [6:7] ${key} (with braces)
// [8:9] $... (Invalid)
var namedFormatMatcher = regexp.MustCompile(
	`\$(?:` +
		`(\$)|` + // Escaped ($$)
		`(` + idPattern + `)|` + // Without braces: $key
		`(?:\{(` + idPattern + `)\})|` + // With braces: ${key}
		`(.*)` + // Invalid
		`)`)

// Resolve resolves substitutions in v using the supplied substitution map,
// subst.
//
// A substitution can have the form:
//
//	$key
//	${key}
//
// The substitution can also be escaped using a second "$", such as "$$".
//
// If the string includes an erroneous substitution, or if a referenced
// template variable isn't included in the "substitutions" map, Resolve will
// return an error.
func Resolve(v string, subst map[string]string) (string, error) {
	smi := namedFormatMatcher.FindAllStringSubmatchIndex(v, -1)
	if len(smi) == 0 {
		// No substitutions.
		return v, nil
	}

	var (
		parts = make([]string, 0, (len(smi)*2)+1)
		pos   = 0
	)

	for _, match := range smi {
		key := ""
		switch {
		case match[8] >= 0:
			// Invalid.
			return "", errors.Fmt("invalid template: %q", v)

		case match[2] >= 0:
			// Escaped.
			parts = append(parts, v[pos:match[2]])
			pos = match[3]
			continue

		case match[4] >= 0:
			// Key (without braces)
			key = v[match[4]:match[5]]
		case match[6] >= 0:
			// Key (with braces)
			key = v[match[6]:match[7]]

		default:
			panic("impossible")
		}

		// Add anything in between the previous match and the current. If our match
		// includes a non-escape character, add that too.
		if pos < match[0] {
			parts = append(parts, v[pos:match[0]])
		}
		pos = match[1]

		subst, ok := subst[key]
		if !ok {
			return "", errors.Fmt("no substitution for %q", key)
		}
		parts = append(parts, subst)
	}

	// Append any trailing string.
	parts = append(parts, v[pos:])

	// Join the parts.
	return strings.Join(parts, ""), nil
}
