// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIndexDefinition(t *testing.T) {
	t.Parallel()

	Convey("Test IndexDefinition", t, func() {
		Convey("basic", func() {
			id := IndexDefinition{Kind: "kind"}

			So(id.Builtin(), ShouldBeTrue)
			So(id.Compound(), ShouldBeFalse)
			So(id.String(), ShouldEqual, "B:kind")

			id.SortBy = append(id.SortBy, IndexColumn{Property: "prop"})
			So(id.SortBy[0].Direction, ShouldEqual, ASCENDING)
			So(id.Builtin(), ShouldBeTrue)
			So(id.Compound(), ShouldBeFalse)
			So(id.String(), ShouldEqual, "B:kind/prop")

			id.SortBy = append(id.SortBy, IndexColumn{"other", DESCENDING})
			id.Ancestor = true
			So(id.Builtin(), ShouldBeFalse)
			So(id.Compound(), ShouldBeTrue)
			So(id.String(), ShouldEqual, "C:kind|A/prop/-other")

			// invalid
			id.SortBy = append(id.SortBy, IndexColumn{"", DESCENDING})
			So(id.Builtin(), ShouldBeFalse)
			So(id.Compound(), ShouldBeFalse)
		})

		Convey("binary", func() {
			id := IndexDefinition{Kind: "kind"}
			buf := &bytes.Buffer{}
			So(id.Write(buf), ShouldBeNil)
			So(bytes.HasPrefix(buf.Bytes(), IndexBuiltinQueryPrefix()), ShouldBeTrue)
			newId := IndexDefinition{}
			So(newId.Read(buf), ShouldBeNil)
			So(newId, ShouldResemble, id)

			id.SortBy = append(id.SortBy, IndexColumn{Property: "prop"})
			buf = &bytes.Buffer{}
			So(id.Write(buf), ShouldBeNil)
			So(bytes.HasPrefix(buf.Bytes(), IndexBuiltinQueryPrefix()), ShouldBeTrue)
			newId = IndexDefinition{}
			So(newId.Read(buf), ShouldBeNil)
			So(newId, ShouldResemble, id)

			id.SortBy = append(id.SortBy, IndexColumn{"other", DESCENDING})
			id.Ancestor = true
			buf = &bytes.Buffer{}
			So(id.Write(buf), ShouldBeNil)
			So(bytes.HasPrefix(buf.Bytes(), IndexComplexQueryPrefix()), ShouldBeTrue)
			newId = IndexDefinition{}
			So(newId.Read(buf), ShouldBeNil)
			So(newId, ShouldResemble, id)

			// invalid
			id.SortBy = append(id.SortBy, IndexColumn{"", DESCENDING})
			buf = &bytes.Buffer{}
			So(id.Write(buf), ShouldBeNil)
			So(bytes.HasPrefix(buf.Bytes(), IndexComplexQueryPrefix()), ShouldBeTrue)
			newId = IndexDefinition{}
			So(newId.Read(buf), ShouldBeNil)
			So(newId, ShouldResemble, id)
		})

		Convey("too many", func() {
			id := IndexDefinition{Kind: "wat"}
			for i := 0; i < MaxIndexColumns+1; i++ {
				id.SortBy = append(id.SortBy, IndexColumn{"Hi", ASCENDING})
			}
			buf := &bytes.Buffer{}
			So(id.Write(buf), ShouldBeNil)
			newId := IndexDefinition{}
			So(newId.Read(buf).Error(), ShouldContainSubstring, "over 64 sort orders")
		})
	})
}
