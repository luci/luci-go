// Copyright 2021 The LUCI Authors.
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

package structmask

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseElement(t *testing.T) {
	t.Parallel()

	ftt.Run("Field", t, func(t *ftt.Test) {
		elem, err := parseElement("abc.def")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"abc.def"}))

		elem, err = parseElement("abc.de\"f")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"abc.de\"f"}))

		elem, err = parseElement("\"abc.def\"")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"abc.def"}))

		elem, err = parseElement("'a'")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"a"}))

		elem, err = parseElement("\"*\"")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"*"}))

		elem, err = parseElement("\"10\"")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"10"}))

		elem, err = parseElement("`'`")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"'"}))

		elem, err = parseElement("")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{""}))

		elem, err = parseElement("/")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(fieldElement{"/"}))

		_, err = parseElement("'not closed")
		assert.Loosely(t, err, should.ErrLike(`bad quoted string`))
	})

	ftt.Run("Index", t, func(t *ftt.Test) {
		elem, err := parseElement("0")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(indexElement{0}))

		elem, err = parseElement("+10")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(indexElement{10}))

		_, err = parseElement("10.0")
		assert.Loosely(t, err, should.ErrLike(`an index must be a non-negative integer`))

		_, err = parseElement("-10")
		assert.Loosely(t, err, should.ErrLike(`an index must be a non-negative integer`))
	})

	ftt.Run("Star", t, func(t *ftt.Test) {
		elem, err := parseElement("*")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, elem, should.Resemble(starElement{}))

		_, err = parseElement("p*")
		assert.Loosely(t, err, should.ErrLike(`prefix and suffix matches are not supported`))
	})

	ftt.Run("/.../", t, func(t *ftt.Test) {
		_, err := parseElement("//")
		assert.Loosely(t, err, should.ErrLike(`regexp matches are not supported`))
	})
}

func TestParseMask(t *testing.T) {
	t.Parallel()

	ftt.Run("OK", t, func(t *ftt.Test) {
		filter, err := NewFilter([]*StructMask{
			{Path: []string{"a", "b1", "c"}},
			{Path: []string{"a", "*", "d"}},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, filter, should.NotBeNil)
	})

	ftt.Run("Bad selector", t, func(t *ftt.Test) {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"*", "-10"}},
		})
		assert.Loosely(t, err, should.ErrLike(`bad element "-10" in the mask ["*","-10"]`))
	})

	ftt.Run("Empty mask", t, func(t *ftt.Test) {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"a", "b1", "c"}},
			{Path: []string{}},
		})
		assert.Loosely(t, err, should.ErrLike("bad empty mask"))
	})

	ftt.Run("Index selector ", t, func(t *ftt.Test) {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"a", "1", "*"}},
		})
		assert.Loosely(t, err, should.ErrLike("individual index selectors are not supported"))
	})
}
