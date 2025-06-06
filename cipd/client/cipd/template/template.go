// Copyright 2018 The LUCI Authors.
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

// Package template implements handling of package name templates.
//
// Package name templates look like e.g. "foo/${platform}" and code in this
// package knows how to expand them into full package names.
package template

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// Expander is a mapping of simple string substitutions which is used to
// expand cipd package name templates. For example:
//
//	ex, err := template.Expander{
//	  "platform": "mac-amd64"
//	}.Expand("foo/${platform}")
//
// `ex` would be "foo/mac-amd64".
//
// Use DefaultExpander() to obtain the default mapping for CIPD
// applications.
type Expander map[string]string

// ErrSkipTemplate may be returned from Expander.Expand to indicate that
// a given expansion doesn't apply to the current template parameters. For
// example, expanding `"foo/${os=linux,mac}"` with a template parameter of "os"
// == "win", would return ErrSkipTemplate.
var ErrSkipTemplate = errors.New("package template does not apply to the current system")

var templateParm = regexp.MustCompile(`\${[^}]*}`)

// Expand applies package template expansion rules to the package template,
//
// If err == ErrSkipTemplate, that means that this template does not apply to
// this os/arch combination and should be skipped.
//
// The expansion rules are as follows:
//   - "some text" will pass through unchanged
//   - "${variable}" will directly substitute the given variable
//   - "${variable=val1,val2}" will substitute the given variable, if its value
//     matches one of the values in the list of values. If the current value
//     does not match, this returns ErrSkipTemplate.
//
// Attempting to expand an unknown variable is an error.
// After expansion, any lingering '$' in the template is an error.
func (t Expander) Expand(template string) (pkg string, err error) {
	return t.expandImpl(template, false)
}

// Validate returns an error if this template doesn't appear to be valid given
// the current Expander parameters.
//
// This will catch issues like malformed template parameters and unknown
// variables, and will replace all ${param=value} items with the first item in
// the value list, even if the current TemplateExpander value doesn't match.
//
// This is mostly used for validating user input when the correct values of
// Expander aren't known yet.
func (t Expander) Validate(template string) (pkg string, err error) {
	return t.expandImpl(template, true)
}

func (t Expander) expandImpl(template string, alwaysFill bool) (pkg string, err error) {
	skip := false

	pkg = templateParm.ReplaceAllStringFunc(template, func(parm string) string {
		// ${...}
		contents := parm[2 : len(parm)-1]

		varNameValues := strings.SplitN(contents, "=", 2)
		if len(varNameValues) == 1 {
			// ${varName}
			if value, ok := t[varNameValues[0]]; ok {
				return value
			}

			err = cipderr.BadArgument.Apply(errors.Fmt("unknown variable in ${%s}", contents))
		}

		// ${varName=value,value}
		ourValue, ok := t[varNameValues[0]]
		if !ok {
			err = cipderr.BadArgument.Apply(errors.Fmt("unknown variable %q", parm))
			return parm
		}

		for _, val := range strings.Split(varNameValues[1], ",") {
			if val == ourValue || alwaysFill {
				return ourValue
			}
		}
		skip = true
		return parm
	})
	if skip {
		err = ErrSkipTemplate
	}
	if err == nil && strings.ContainsRune(pkg, '$') {
		err = cipderr.BadArgument.Apply(errors.Fmt("unable to process some variables in %q", template))
	}
	return
}

// Platform contains the parameters for a "${platform}" template.
//
// The string value can be obtained by calling String().
// be parsed using ParsePlatform.
type Platform struct {
	OS   string
	Arch string
}

// ParsePlatform parses a Platform from its string representation.
func ParsePlatform(v string) (Platform, error) {
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return Platform{}, cipderr.BadArgument.Apply(errors.Fmt("platform must be <os>-<arch>, got %q", v))
	}
	return Platform{parts[0], parts[1]}, nil
}

func (tp Platform) String() string {
	return fmt.Sprintf("%s-%s", tp.OS, tp.Arch)
}

// Expander returns an Expander populated with tp's fields.
func (tp Platform) Expander() Expander {
	return Expander{
		"os":       tp.OS,
		"arch":     tp.Arch,
		"platform": tp.String(),
	}
}

// DefaultTemplate returns the default platform template.
//
// This has values populated for ${os}, ${arch} and ${platform}.
func DefaultTemplate() Platform {
	return Platform{platform.CurrentOS(), platform.CurrentArchitecture()}
}

// DefaultExpander returns the default template expander.
//
// This has values populated for ${os}, ${arch} and ${platform}.
func DefaultExpander() Expander {
	return DefaultTemplate().Expander()
}
