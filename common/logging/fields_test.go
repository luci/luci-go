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

package logging_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/luci/common/logging"
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
		fe := logging.FieldEntry{Key: "value", Value: `"Hello, World!"`}

		t.Run(`Has a String() value, "value":"\"Hello, World!\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"\"Hello, World!\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => 42`, t, func(t *ftt.Test) {
		fe := logging.FieldEntry{Key: "value", Value: 42}

		t.Run(`Has a String() value, "value":"42".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":42`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => stringStruct{"My \"value\""}`, t, func(t *ftt.Test) {
		fe := logging.FieldEntry{Key: "value", Value: &stringStruct{`My "value"`}}

		t.Run(`Has a String() value, "value":"My \"value\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"My \"value\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => error{"There was a \"failure\"}`, t, func(t *ftt.Test) {
		fe := logging.FieldEntry{Key: "value", Value: errors.New(`There was a "failure"`)}

		t.Run(`Has a String() value, "value":"There was a \"failure\"".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":"There was a \"failure\""`))
		})
	})

	ftt.Run(`A FieldEntry instance: "value" => struct{a: "Hello!", b: 42}`, t, func(t *ftt.Test) {
		type myStruct struct {
			a string
			b int
		}
		fe := logging.FieldEntry{Key: "value", Value: &myStruct{"Hello!", 42}}

		t.Run(`Has a String() value, "value":myStruct { a: "Hello!", b: 42 }".`, func(t *ftt.Test) {
			assert.Loosely(t, fe.String(), should.Equal(`"value":&logging_test.myStruct{a:"Hello!", b:42}`))
		})
	})

	ftt.Run(`A SortedEntries: {foo/bar, error/z, asdf/baz}`, t, func(t *ftt.Test) {
		zErr := errors.New("z")
		fields := logging.Fields{
			"foo":            "bar",
			logging.ErrorKey: zErr,
			"asdf":           "baz",
		}

		t.Run(`Should be sorted: [error, asdf, foo].`, func(t *ftt.Test) {
			assert.That(t, fields.SortedEntries(), should.Match(
				[]*logging.FieldEntry{
					{Key: logging.ErrorKey, Value: zErr},
					{Key: "asdf", Value: "baz"},
					{Key: "foo", Value: "bar"},
				},
				cmpopts.EquateErrors()))
		})
	})
}

func TestFields(t *testing.T) {
	ftt.Run(`A nil Fields`, t, func(t *ftt.Test) {
		fm := logging.Fields(nil)

		t.Run(`Returns nil when Copied with an empty Fields.`, func(t *ftt.Test) {
			assert.Loosely(t, fm.Copy(logging.Fields{}), should.BeNil)
		})

		t.Run(`Returns a populated Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			other := logging.Fields{
				"foo": "bar",
				"baz": "qux",
			}
			assert.Loosely(t, fm.Copy(other), should.Match(logging.Fields{"foo": "bar", "baz": "qux"}))
		})

		t.Run(`Returns the populated Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			other := logging.Fields{
				"foo": "bar",
				"baz": "qux",
			}
			assert.Loosely(t, fm.Copy(other), should.Match(other))
		})
	})

	ftt.Run(`A populated Fields`, t, func(t *ftt.Test) {
		fm := logging.NewFields(map[string]any{
			"foo": "bar",
			"baz": "qux",
		})
		assert.Loosely(t, fm, should.HaveType[logging.Fields])

		t.Run(`Returns an augmented Fields when Copied with a populated Fields.`, func(t *ftt.Test) {
			err := errors.New("err")
			other := logging.Fields{logging.ErrorKey: err}
			assert.Loosely(t, fm.Copy(other), should.Match(
				logging.Fields{"foo": "bar", "baz": "qux", logging.ErrorKey: err},
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
			assert.Loosely(t, logging.GetFields(c), should.BeNil)
		})

		t.Run(`Sets {"foo": "bar", "baz": "qux"}`, func(t *ftt.Test) {
			c = logging.SetFields(c, logging.Fields{
				"foo": "bar",
				"baz": "qux",
			})
			assert.Loosely(t, logging.GetFields(c), should.Match(logging.Fields{
				"foo": "bar",
				"baz": "qux",
			}))

			t.Run(`Is overridden by: {"foo": "override", "error": "failure"}`, func(t *ftt.Test) {
				err := errors.New("failure")
				c = logging.SetFields(c, logging.Fields{
					"foo":            "override",
					logging.ErrorKey: err,
				})

				assert.Loosely(t, logging.GetFields(c), should.Match(logging.Fields{
					"foo":            "override",
					"baz":            "qux",
					logging.ErrorKey: err,
				}, cmpopts.EquateErrors()))
			})
		})
	})
}
