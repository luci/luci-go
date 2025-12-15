// Copyright 2025 The LUCI Authors.
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

package aip160

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestArgParsers(t *testing.T) {
	ftt.Run("ArgParsers", t, func(t *ftt.Test) {
		t.Run("CoerceArgToBoolConstant", func(t *ftt.Test) {
			cases := []struct {
				name      string
				filter    string
				expected  bool
				errString string
			}{
				{
					name:     "true",
					filter:   "foo = true",
					expected: true,
				},
				{
					name:     "false",
					filter:   "foo = false",
					expected: false,
				},
				{
					name:      "quoted true",
					filter:    `foo = "true"`,
					errString: `expected the unquoted literal 'true' or 'false' but found double-quoted string "true"`,
				},
				{
					name:      "quoted false",
					filter:    `foo = "false"`,
					errString: `expected the unquoted literal 'true' or 'false' but found double-quoted string "false"`,
				},
				{
					name:      "invalid value",
					filter:    "foo = bar",
					errString: `expected the unquoted literal 'true' or 'false' (case-sensitive) but found "bar"`,
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *ftt.Test) {
					filter, err := ParseFilter(tc.filter)
					assert.Loosely(t, err, should.BeNil)
					arg := filter.Expression.Sequences[0].Factors[0].Terms[0].Simple.Restriction.Arg
					val, err := CoerceArgToBoolConstant(arg)
					if tc.errString != "" {
						assert.Loosely(t, err, should.ErrLike(tc.errString))
					} else {
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, val, should.Equal(tc.expected))
					}
				})
			}
		})

		t.Run("CoerceArgToStringConstant", func(t *ftt.Test) {
			cases := []struct {
				name      string
				filter    string
				expected  string
				errString string
			}{
				{
					name:     "quoted string",
					filter:   `foo = "bar"`,
					expected: "bar",
				},
				{
					name:     "empty string",
					filter:   `foo = ""`,
					expected: "",
				},
				{
					name:      "unquoted string",
					filter:    "foo = bar",
					errString: `expected a quoted ("") string literal but got possible field reference "bar", did you mean to wrap the value in quotes?`,
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *ftt.Test) {
					filter, err := ParseFilter(tc.filter)
					assert.Loosely(t, err, should.BeNil)
					arg := filter.Expression.Sequences[0].Factors[0].Terms[0].Simple.Restriction.Arg
					val, err := CoerceArgToStringConstant(arg)
					if tc.errString != "" {
						assert.Loosely(t, err, should.ErrLike(tc.errString))
					} else {
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, val, should.Equal(tc.expected))
					}
				})
			}
		})
	})
}
