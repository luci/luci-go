// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

// TestFieldEntry tests methods associated with the FieldEntry and
// fieldEntrySlice types.
func TestFieldEntry(t *testing.T) {
	Convey(`A FieldEntry instance: "value" => "\"Hello, World!\""`, t, func() {
		fe := FieldEntry{"value", `"Hello, World!"`}

		Convey(`Has a String() value, "value":"\"Hello, World!\"".`, func() {
			So(fe.String(), ShouldEqual, `"value":"\"Hello, World!\""`)
		})
	})

	Convey(`A fieldEntrySlice: {foo/bar, error/z, asdf/baz}`, t, func() {
		fes := fieldEntrySlice{
			&FieldEntry{"foo", "bar"},
			&FieldEntry{ErrorKey, "z"},
			&FieldEntry{"asdf", "baz"},
		}

		Convey(`Should be sorted: [error, asdf, foo].`, func() {
			sorted := make(fieldEntrySlice, len(fes))
			copy(sorted, fes)
			sort.Sort(sorted)

			So(sorted, ShouldResemble, fieldEntrySlice{fes[1], fes[2], fes[0]})
		})
	})
}

func TestFields(t *testing.T) {
	Convey(`A nil Fields`, t, func() {
		fm := Fields(nil)

		Convey(`Returns nil when Copied with an empty Fields.`, func() {
			So(fm.Copy(Fields{}), ShouldBeNil)
		})

		Convey(`Returns a populated Fields when Copied with a populated Fields.`, func() {
			other := Fields{
				"foo": "bar",
				"baz": "qux",
			}
			So(fm.Copy(other), ShouldResemble, Fields{"foo": "bar", "baz": "qux"})
		})

		Convey(`Returns the populated Fields when Copied with a populated Fields.`, func() {
			other := Fields{
				"foo": "bar",
				"baz": "qux",
			}
			So(fm.Copy(other), ShouldResemble, other)
		})
	})

	Convey(`A populated Fields`, t, func() {
		fm := NewFields(map[string]interface{}{
			"foo": "bar",
			"baz": "qux",
		})
		So(fm, ShouldHaveSameTypeAs, Fields(nil))

		Convey(`Returns an augmented FieldMap when Copied with a populated Fields.`, func() {
			other := Fields{
				ErrorKey: "err",
			}
			So(fm.Copy(other), ShouldResemble, Fields{"foo": "bar", "baz": "qux", ErrorKey: "err"})
		})
	})
}

func TestContextFields(t *testing.T) {
	Convey(`An empty Context`, t, func() {
		c := context.Background()

		Convey(`Has no Fields.`, func() {
			So(GetFields(c), ShouldBeNil)
		})

		Convey(`Sets {"foo": "bar", "baz": "qux"}`, func() {
			c = SetFields(c, Fields{
				"foo": "bar",
				"baz": "qux",
			})
			So(GetFields(c), ShouldResemble, Fields{
				"foo": "bar",
				"baz": "qux",
			})

			Convey(`Is overridden by: {"foo": "override", "error": "failure"}`, func() {
				c = SetFields(c, Fields{
					"foo":   "override",
					"error": "failure",
				})

				So(GetFields(c), ShouldResemble, Fields{
					"foo":   "override",
					"baz":   "qux",
					"error": "failure",
				})
			})
		})
	})
}
