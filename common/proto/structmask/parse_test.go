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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseElement(t *testing.T) {
	t.Parallel()

	Convey("Field", t, func() {
		elem, err := parseElement("abc.def")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"abc.def"})

		elem, err = parseElement("abc.de\"f")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"abc.de\"f"})

		elem, err = parseElement("\"abc.def\"")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"abc.def"})

		elem, err = parseElement("'a'")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"a"})

		elem, err = parseElement("\"*\"")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"*"})

		elem, err = parseElement("\"10\"")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"10"})

		elem, err = parseElement("`'`")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"'"})

		elem, err = parseElement("")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{""})

		elem, err = parseElement("/")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, fieldElement{"/"})

		_, err = parseElement("'not closed")
		So(err, ShouldErrLike, `bad quoted string`)
	})

	Convey("Index", t, func() {
		elem, err := parseElement("0")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, indexElement{0})

		elem, err = parseElement("+10")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, indexElement{10})

		_, err = parseElement("10.0")
		So(err, ShouldErrLike, `an index must be a non-negative integer`)

		_, err = parseElement("-10")
		So(err, ShouldErrLike, `an index must be a non-negative integer`)
	})

	Convey("Star", t, func() {
		elem, err := parseElement("*")
		So(err, ShouldBeNil)
		So(elem, ShouldResemble, starElement{})

		_, err = parseElement("p*")
		So(err, ShouldErrLike, `prefix and suffix matches are not supported`)
	})

	Convey("/.../", t, func() {
		_, err := parseElement("//")
		So(err, ShouldErrLike, `regexp matches are not supported`)
	})
}

func TestParseMask(t *testing.T) {
	t.Parallel()

	Convey("OK", t, func() {
		filter, err := NewFilter([]*StructMask{
			{Path: []string{"a", "b1", "c"}},
			{Path: []string{"a", "*", "d"}},
		})
		So(err, ShouldBeNil)
		So(filter, ShouldNotBeNil)
	})

	Convey("Bad selector", t, func() {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"*", "-10"}},
		})
		So(err, ShouldErrLike, `bad element "-10" in the mask ["*","-10"]`)
	})

	Convey("Empty mask", t, func() {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"a", "b1", "c"}},
			{Path: []string{}},
		})
		So(err, ShouldErrLike, "bad empty mask")
	})

	Convey("Index selector ", t, func() {
		_, err := NewFilter([]*StructMask{
			{Path: []string{"a", "1", "*"}},
		})
		So(err, ShouldErrLike, "individual index selectors are not supported")
	})
}
