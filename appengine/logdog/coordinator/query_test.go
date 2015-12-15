// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"testing"

	ds "github.com/luci/gae/service/datastore"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

// sps constructs a []ds.Property slice from a single interface.
func sps(values ...interface{}) ds.PropertySlice {
	ps := make(ds.PropertySlice, len(values))
	for i, v := range values {
		ps[i] = ds.MkProperty(v)
	}
	return ps
}

func TestQueryBase(t *testing.T) {
	Convey(`A QueryBase with invalid tags will fail to encode.`, t, func() {
		qb := &QueryBase{
			Tags: TagMap{
				"!!!invalid key!!!": "value",
			},
		}
		So(qb.PLSSaveTo(ds.PropertyMap{}, true), ShouldErrLike, "failed to encode tags")
	})

	Convey(`A QueryBase will skip invalid tags when loading.`, t, func() {
		qb := &QueryBase{}
		pmap := ds.PropertyMap{
			"_Tags": sps(encodeKey("!!!invalid key!!!")),
		}
		So(qb.PLSLoadFrom(pmap), ShouldBeNil)
		So(qb.Tags, ShouldResembleV, TagMap(nil))
	})
}

func TestQueryComponentFilter(t *testing.T) {
	Convey(`A testing query`, t, func() {
		q := ds.NewQuery("QueryBase")

		Convey(`Will construct a non-globbing query as Prefix/Name equality.`, func() {
			q, err := AddLogStreamPathQuery(q, "foo/bar/+/baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"Prefix": sps("foo/bar"),
				"Name":   sps("baz/qux"),
			})
		})

		Convey(`Will refuse to query an invalid Prefix/Name.`, func() {
			_, err := AddLogStreamPathQuery(q, "////+/baz/qux")
			So(err, ShouldErrLike, "invalid Prefix component")

			_, err = AddLogStreamPathQuery(q, "foo/bar/+//////")
			So(err, ShouldErrLike, "invalid Name component")
		})

		Convey(`Will not impose any filters on an empty Prefix.`, func() {
			q, err := AddLogStreamPathQuery(q, "/+/baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"Name": sps("baz/qux"),
			})
		})

		Convey(`Will not impose any filters on an empty Name.`, func() {
			q, err := AddLogStreamPathQuery(q, "baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"Prefix": sps("baz/qux"),
			})
		})

		Convey(`Will glob out single Prefix components.`, func() {
			q, err := AddLogStreamPathQuery(q, "foo/*/*/bar/*/baz/qux/*")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"_Prefix.0":     sps("foo"),
				"_Prefix.3":     sps("bar"),
				"_Prefix.5":     sps("baz"),
				"_Prefix.6":     sps("qux"),
				"_Prefix.count": sps(8),
			})
		})

		Convey(`Will handle end-of-query globbing.`, func() {
			q, err := AddLogStreamPathQuery(q, "foo/*/bar/**")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"_Prefix.0": sps("foo"),
				"_Prefix.2": sps("bar"),
			})
		})

		Convey(`Will handle beginning-of-query globbing.`, func() {
			q, err := AddLogStreamPathQuery(q, "**/foo/*/bar")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"_Prefix.r.0": sps("bar"),
				"_Prefix.r.2": sps("foo"),
			})
		})

		Convey(`Can handle middle-of-query globbing.`, func() {
			q, err := AddLogStreamPathQuery(q, "*/foo/*/**/bar/*/baz/*")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResembleV, map[string]ds.PropertySlice{
				"_Prefix.1":   sps("foo"),
				"_Prefix.r.1": sps("baz"),
				"_Prefix.r.3": sps("bar"),
			})
		})

		Convey(`Will error if more than one greedy glob is present.`, func() {
			_, err := AddLogStreamPathQuery(q, "*/foo/**/bar/**")
			So(err, ShouldErrLike, "cannot have more than one greedy glob")
		})
	})
}
