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

package logging

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type stringStruct struct {
	Value string
}

var _ fmt.Stringer = (*stringStruct)(nil)

func (s *stringStruct) String() string {
	return s.Value
}

// TestFieldEntry tests methods associated with the FieldEntry and
// fieldEntrySlice types.
func TestFieldEntry(t *testing.T) {
	ftt.Run(`A FieldEntry instance: "value" => "\"Hello, World!\""`, t, func(t *ftt.Test) {
		fe := FieldEntry{"value", `"Hello, World!"`}

		t.Run(`Has a String() value, "value":"\"Hello, World!\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"\"Hello, World!\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => 42`, t, func(t *ftt.Test) {
		fe := FieldEntry{"value", 42}

		t.Run(`Has a String() value, "value":"42".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":42`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => stringStruct{"My \"value\""}`, t, func(t *ftt.Test) {
		fe := FieldEntry{"value", &stringStruct{`My "value"`}}

		t.Run(`Has a String() value, "value":"My \"value\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"My \"value\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => error{"There was a \"failure\"}`, t, func(t *ftt.Test) {
		fe := FieldEntry{"value", errors.New(`There was a "failure"`)}

		t.Run(`Has a String() value, "value":"There was a \"failure\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"There was a \"failure\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => struct{a: "Hello!", b: 42}`, t, func(t *ftt.Test) {
		type myStruct struct {
			a string
			b int
		}
		fe := FieldEntry{"value", &myStruct{"Hello!", 42}}

		t.Run(`Has a String() value, "value":myStruct { a: "Hello!", b: 42 }".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":&logging.myStruct{a:"Hello!", b:42}`))
		})
	})

	ftt.Run(`A fieldEntrySlice: {foo/bar, error/z, asdf/baz}`, t, func(t *ftt.Test) {
		zErr := errors.New("z")
		fes := fieldEntrySlice{
			&FieldEntry{"foo", "bar"},
			&FieldEntry{ErrorKey, zErr},
			&FieldEntry{"asdf", "baz"},
		}

		t.Run(`Should be sorted: [error, asdf, foo].`, func(t *ftt.Test) {
			sorted := make(fieldEntrySlice, len(fes))
			copy(sorted, fes)
			sort.Sort(sorted)

			assert.Loosely(t, sorted, should.Match(
				fieldEntrySlice{fes[1], fes[2], fes[0]},
				cmpopts.EquateErrors()))
		})
	})
}

func TestFields(t *testing.T) {
	ftt.Run(`A nil Fields`, t, func(t *ftt.Test) {
		fm := Fields(nil)

		t.Run(`Returns nil when Copied with an empty Fields.`, func(t *ftt.Test) {
			assert.Loosely(t, fm.Copy(Fields{}), should.BeNil)
		})

		t.Run(`Returns a populated Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			other := Fields{
				"foo": "bar",
				"baz": "qux",
			}
			assert.Loosely(t, fm.Copy(other), should.Resemble(Fields{"foo": "bar", "baz": "qux"}))
		})

		t.Run(`Returns the populated Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			other := Fields{
				"foo": "bar",
				"baz": "qux",
			}
			assert.Loosely(t, fm.Copy(other), should.Match(
				other, cmpopts.EquateErrors()))
		})
	})

	ftt.Run(`A populated Fields`, t, func(t *ftt.Test) {
		fm := NewFields(map[string]any{
			"foo": "bar",
			"baz": "qux",
		})
		assert.Loosely(t, fm, should.HaveType[Fields])

		t.Run(`Returns an augmented Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			err := errors.New("err")
			other := Fields{ErrorKey: err}
			assert.Loosely(t, fm.Copy(other), should.Match(
				Fields{"foo": "bar", "baz": "qux", ErrorKey: err},
				cmpopts.EquateErrors()))
		})

		t.Run(`Has a String representation: {"baz":"qux", "foo":"bar"}`, func(t *ftt.Test) {
			assert.Loosely(t, fm.String(), should.Equal(`{"baz":"qux", "foo":"bar"}`))
		})
	})
}

func TestContextFields(t *testing.T) {
	ftt.Run(`An empty Context`, t, func(t *ftt.Test) {
		c := context.Background()

		t.Run(`Has no Fields.`, func(t *ftt.Test) {
			assert.Loosely(t, GetFields(c), should.BeNil)
		})

		t.Run(`Sets {"foo": "bar", "baz": "qux"}`, func(t *ftt.Test) {
			c = SetFields(c, Fields{
				"foo": "bar",
				"baz": "qux",
			})
			assert.Loosely(t, GetFields(c), should.Resemble(Fields{
				"foo": "bar",
				"baz": "qux",
			}))

			t.Run(`Is overridden by: {"foo": "override", "error": "failure"}`, func(t *ftt.Test) {
				err := errors.New("failure")
				c = SetFields(c, Fields{
					"foo":   "override",
					"error": err,
				})

				assert.Loosely(t, GetFields(c), should.Match(Fields{
					"foo":   "override",
					"baz":   "qux",
					"error": err,
				}, cmpopts.EquateErrors()))
			})
		})
	})
}
