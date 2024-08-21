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

package template

import (
	"fmt"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestTemplateExpander(t *testing.T) {
	t.Parallel()

	tests := []struct {
		template string
		expected string
		err      string
	}{
		// ok
		{"", "", ""},
		{"${os}${os}", "SOME_OSSOME_OS", ""},
		{"Something", "Something", ""},
		{"Something/${os}/moo", "Something/SOME_OS/moo", ""},
		{"Something/${os}-${arch}", "Something/SOME_OS-SOME_ARCH", ""},
		{"Something/${platform}", "Something/SOME_OS-SOME_ARCH", ""},
		{"Something/${os=wut,SOME_OS}", "Something/SOME_OS", ""},
		{"Something/${os=wut,SOME_OS}-${arch}", "Something/SOME_OS-SOME_ARCH", ""},

		// errors
		{"Something/${var}", "", "unknown variable"},
		{"Something/${os}-${arch=deez}", "", ErrSkipTemplate.Error()},
		{"Something/${platform=deez}", "", ErrSkipTemplate.Error()},
		{"Something/${platform}$", "", "unable to process some variables"},
		{"Something/${platform${os}}", "", "unknown variable"},
		{"Something/${platform", "", "unable to process some variables"},
		{"Something/$platform", "", "unable to process some variables"},
	}

	expander := Expander{
		"os":       "SOME_OS",
		"arch":     "SOME_ARCH",
		"platform": "SOME_OS-SOME_ARCH",
	}

	ftt.Run(`TemplateExpander`, t, func(t *ftt.Test) {
		for _, tc := range tests {
			expect := ""
			if tc.err == "" {
				expect = fmt.Sprintf("%q", tc.expected)
			} else {
				expect = fmt.Sprintf("err(%q)", tc.err)
			}
			t.Run(fmt.Sprintf(`%q -> %s`, tc.template, expect), func(t *ftt.Test) {
				val, err := expander.Expand(tc.template)
				if tc.err != "" {
					assert.Loosely(t, err, should.ErrLike(tc.err))
				} else {
					assert.Loosely(t, val, should.Equal(tc.expected))
				}
			})
		}
	})
}
