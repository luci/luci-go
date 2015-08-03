// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"testing"

	"github.com/luci/gae/service/info"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type fakeRDS struct{ RawInterface }

func (fakeRDS) NewQuery(string) Query { return &fakeQuery{} }

func TestCheckFilter(t *testing.T) {
	t.Parallel()

	Convey("Test checkFilter", t, func() {
		// Note that the way we have this context set up, any calls which aren't
		// stopped at the checkFilter will nil-pointer panic. We use this panic
		// behavior to indicate that the checkfilter has allowed a call to pass
		// through to the implementation in the tests below. In a real application
		// the panics observed in the tests below would actually be sucessful calls
		// to the implementation.
		c := SetRaw(info.Set(context.Background(), fakeInfo{}), fakeRDS{})
		rds := GetRaw(c) // has checkFilter
		So(rds, ShouldNotBeNil)

		Convey("RunInTransaction", func() {
			So(rds.RunInTransaction(nil, nil).Error(), ShouldContainSubstring, "is nil")
			hit := false
			So(func() {
				rds.RunInTransaction(func(context.Context) error {
					hit = true
					return nil
				}, nil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("Run", func() {
			So(rds.Run(nil, nil).Error(), ShouldContainSubstring, "query is nil")
			So(rds.Run(rds.NewQuery("sup"), nil).Error(), ShouldContainSubstring, "callback is nil")
			hit := false
			So(func() {
				rds.Run(rds.NewQuery("sup"), func(Key, PropertyMap, func() (Cursor, error)) bool {
					hit = true
					return true
				})
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("GetMulti", func() {
			So(rds.GetMulti(nil, nil), ShouldBeNil)
			So(rds.GetMulti([]Key{NewKey("", "", "", "", 0, nil)}, nil).Error(), ShouldContainSubstring, "is nil")

			// this is in the wrong aid/ns
			keys := []Key{NewKey("wut", "wrong", "Kind", "", 1, nil)}
			So(rds.GetMulti(keys, func(pm PropertyMap, err error) {
				So(pm, ShouldBeNil)
				So(err, ShouldEqual, ErrInvalidKey)
			}), ShouldBeNil)

			keys[0] = NewKey("aid", "ns", "Kind", "", 1, nil)
			hit := false
			So(func() {
				rds.GetMulti(keys, func(pm PropertyMap, err error) {
					hit = true
				})
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("PutMulti", func() {
			keys := []Key{}
			vals := []PropertyMap{{}}
			So(rds.PutMulti(keys, vals, nil).Error(),
				ShouldContainSubstring, "mismatched keys/vals")
			So(rds.PutMulti(nil, nil, nil), ShouldBeNil)

			badParent := NewKey("aid", "ns", "Wut", "", 0, nil)
			keys = append(keys, NewKey("aid", "ns", "Kind", "", 0, badParent))
			So(rds.PutMulti(keys, vals, nil).Error(), ShouldContainSubstring, "callback is nil")

			So(rds.PutMulti(keys, vals, func(k Key, err error) {
				So(k, ShouldBeNil)
				So(err, ShouldEqual, ErrInvalidKey)
			}), ShouldBeNil)

			keys = []Key{NewKey("aid", "ns", "Kind", "", 0, nil)}
			vals = []PropertyMap{nil}
			So(rds.PutMulti(keys, vals, func(k Key, err error) {
				So(k, ShouldBeNil)
				So(err.Error(), ShouldContainSubstring, "nil vals entry")
			}), ShouldBeNil)

			vals = []PropertyMap{{}}
			hit := false
			So(func() {
				rds.PutMulti(keys, vals, func(k Key, err error) {
					hit = true
				})
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("DeleteMulti", func() {
			So(rds.DeleteMulti(nil, nil), ShouldBeNil)
			So(rds.DeleteMulti([]Key{NewKey("", "", "", "", 0, nil)}, nil).Error(), ShouldContainSubstring, "is nil")
			So(rds.DeleteMulti([]Key{NewKey("", "", "", "", 0, nil)}, func(err error) {
				So(err, ShouldEqual, ErrInvalidKey)
			}), ShouldBeNil)

			hit := false
			So(func() {
				rds.DeleteMulti([]Key{NewKey("aid", "ns", "Kind", "", 1, nil)}, func(error) {
					hit = true
				})
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

	})
}
