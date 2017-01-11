// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"testing"

	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeRDS struct{ RawInterface }

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
		rds := Raw(c) // has checkFilter
		So(rds, ShouldNotBeNil)

		Convey("RunInTransaction", func() {
			So(rds.RunInTransaction(nil, nil).Error(), ShouldContainSubstring, "is nil")
			hit := false
			So(func() {
				So(rds.RunInTransaction(func(context.Context) error {
					hit = true
					return nil
				}, nil), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("Run", func() {
			So(rds.Run(nil, nil).Error(), ShouldContainSubstring, "query is nil")
			fq, err := NewQuery("sup").Finalize()
			So(err, ShouldBeNil)

			So(rds.Run(fq, nil).Error(), ShouldContainSubstring, "callback is nil")
			hit := false
			So(func() {
				So(rds.Run(fq, func(*Key, PropertyMap, CursorCB) error {
					hit = true
					return nil
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("GetMulti", func() {
			So(rds.GetMulti(nil, nil, nil), ShouldBeNil)
			So(rds.GetMulti([]*Key{mkKey("", "", "", "")}, nil, nil).Error(), ShouldContainSubstring, "is nil")

			// this is in the wrong aid/ns
			keys := []*Key{MkKeyContext("wut", "wrong").MakeKey("Kind", 1)}
			So(rds.GetMulti(keys, nil, func(pm PropertyMap, err error) error {
				So(pm, ShouldBeNil)
				So(IsErrInvalidKey(err), ShouldBeTrue)
				return nil
			}), ShouldBeNil)

			keys[0] = mkKey("Kind", 1)
			hit := false
			So(func() {
				So(rds.GetMulti(keys, nil, func(pm PropertyMap, err error) error {
					hit = true
					return nil
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("PutMulti", func() {
			keys := []*Key{}
			vals := []PropertyMap{{}}
			So(rds.PutMulti(keys, vals, nil).Error(),
				ShouldContainSubstring, "mismatched keys/vals")
			So(rds.PutMulti(nil, nil, nil), ShouldBeNil)

			keys = append(keys, mkKey("aid", "ns", "Wut", 0, "Kind", 0))
			So(rds.PutMulti(keys, vals, nil).Error(), ShouldContainSubstring, "callback is nil")

			So(rds.PutMulti(keys, vals, func(k *Key, err error) error {
				So(k, ShouldBeNil)
				So(IsErrInvalidKey(err), ShouldBeTrue)
				return nil
			}), ShouldBeNil)

			keys = []*Key{mkKey("s~aid", "ns", "Kind", 0)}
			vals = []PropertyMap{nil}
			So(rds.PutMulti(keys, vals, func(k *Key, err error) error {
				So(k, ShouldBeNil)
				So(err.Error(), ShouldContainSubstring, "nil vals entry")
				return nil
			}), ShouldBeNil)

			vals = []PropertyMap{{}}
			hit := false
			So(func() {
				So(rds.PutMulti(keys, vals, func(k *Key, err error) error {
					hit = true
					return nil
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("DeleteMulti", func() {
			So(rds.DeleteMulti(nil, nil), ShouldBeNil)
			So(rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, nil).Error(), ShouldContainSubstring, "is nil")
			So(rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, func(err error) error {
				So(IsErrInvalidKey(err), ShouldBeTrue)
				return nil
			}), ShouldBeNil)

			hit := false
			So(func() {
				So(rds.DeleteMulti([]*Key{mkKey("s~aid", "ns", "Kind", 1)}, func(error) error {
					hit = true
					return nil
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

	})
}
