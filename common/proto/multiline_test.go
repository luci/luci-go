// Copyright 2015 The LUCI Authors.
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

package proto

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMultiline(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name, expect, data string
	}{
		{"basic", `something: "this\000 \t is\n\na \"basic\" test\\example"`, `something: << EOF
		  this` + "\x00" + ` 	 is

		  a "basic" test\example
		EOF`},
		{"contained", `something: "A << B"`, `something: "A << B"`},
		{"indent", `something: "this\n  is indented\n\nwith empty line"`, `something: << EOF
			this
			  is indented
	` + /* this prevents editors from eating whitespace */ `
			with empty line
		EOF`},
		{"col 0 align", `something: "this\n  is indented\n\nwith empty line"`, `something: << EOF
this
  is indented

with empty line
		EOF`},
		{"nested", `something: "<< nerp\nOther\nnerp\nfoo"`, `something: << EOF
		<< nerp
		Other
		nerp
		foo
		EOF`},
		{"multi", `something: "this is something"
			else: "this is else"`, `something: << EOF
		this is something
		EOF
			else: << ELSE
			this is else
			ELSE`},
		{"indented first line", `something: "  this line\nis indented\n  this too"`, `something: <<DERP
		  this line
		is indented
		  this too
		DERP`},
		{"mixed indents are not indents", `something: "\ttab\n  spaces"`, `something: <<DERP
			tab
		  spaces
		DERP`},
	}

	ftt.Run("Test ParseMultilineStrings", t, func(t *ftt.Test) {
		for _, tc := range tcs {
			t.Run(tc.name, func(t *ftt.Test) {
				data, err := ParseMultilineStrings(tc.data)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, data, should.Equal(tc.expect))
			})
		}

		t.Run("missing terminator", func(t *ftt.Test) {
			_, err := ParseMultilineStrings(`<<DERP
			Some stuff
			`)
			assert.Loosely(t, err, should.ErrLike(`failed to find matching terminator "DERP"`))
		})
	})
}
