// Copyright 2022 The LUCI Authors.
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

package aip

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseOrderBy(t *testing.T) {
	ftt.Run("ParseOrderBy", t, func(t *ftt.Test) {
		// Test examples from the AIP-132 spec.
		t.Run("Values should be a comma separated list of fields", func(t *ftt.Test) {
			result, err := ParseOrderBy("foo,bar")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath: NewFieldPath("bar"),
				},
			}))

			result, err = ParseOrderBy("foo")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
			}))
		})
		t.Run("The default sort order is ascending", func(t *ftt.Test) {
			result, err := ParseOrderBy("foo desc, bar")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath:  NewFieldPath("foo"),
					Descending: true,
				},
				{
					FieldPath: NewFieldPath("bar"),
				},
			}))
		})
		t.Run("Redundant space characters in the syntax are insignificant", func(t *ftt.Test) {
			expectedResult := []OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath:  NewFieldPath("bar"),
					Descending: true,
				},
			}
			result, err := ParseOrderBy("foo, bar desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))

			result, err = ParseOrderBy("  foo  ,  bar desc  ")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))

			result, err = ParseOrderBy("foo,bar desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
		t.Run("Subfields are specified with a . character", func(t *ftt.Test) {
			result, err := ParseOrderBy("foo.bar, foo.foo.bar desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath: NewFieldPath("foo", "bar"),
				},
				{
					FieldPath:  NewFieldPath("foo", "foo", "bar"),
					Descending: true,
				},
			}))
		})
		t.Run("Quoted strings can be used instead of string literals", func(t *ftt.Test) {
			result, err := ParseOrderBy("foo.`bar`, foo.foo.`a-backtick-```.bar desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath: NewFieldPath("foo", "bar"),
				},
				{
					FieldPath:  NewFieldPath("foo", "foo", "a-backtick-`", "bar"),
					Descending: true,
				},
			}))
		})
		t.Run("Invalid input is rejected", func(t *ftt.Test) {
			_, err := ParseOrderBy("`something")
			assert.Loosely(t, err, should.ErrLike("syntax error: 1:1: invalid input text \"`something\""))
		})
		t.Run("Empty order by", func(t *ftt.Test) {
			t.Run("Spaces only", func(t *ftt.Test) {
				result, err := ParseOrderBy("   ")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.HaveLength(0))
			})
			t.Run("Totally empty", func(t *ftt.Test) {
				result, err := ParseOrderBy("")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.HaveLength(0))
			})
		})
	})
}
