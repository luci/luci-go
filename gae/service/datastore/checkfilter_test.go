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

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"testing"

	"go.chromium.org/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type fakeRDS struct{ RawInterface }

func (fakeRDS) Constraints() Constraints { return Constraints{} }

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
			So(rds.RunInTransaction(nil, nil), ShouldErrLike, "is nil")
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
			So(rds.Run(nil, nil), ShouldErrLike, "query is nil")
			fq, err := NewQuery("sup").Finalize()
			So(err, ShouldBeNil)

			So(rds.Run(fq, nil), ShouldErrLike, "callback is nil")
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
			So(rds.GetMulti(nil, nil, func(int, PropertyMap, error) {}), ShouldBeNil)
			So(rds.GetMulti([]*Key{mkKey("", "", "", "")}, nil, nil), ShouldErrLike, "is nil")

			// this is in the wrong aid/ns
			keys := []*Key{MkKeyContext("wut", "wrong").MakeKey("Kind", 1)}
			So(rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {
				So(pm, ShouldBeNil)
				So(IsErrInvalidKey(err), ShouldBeTrue)
			}), ShouldBeNil)

			keys[0] = mkKey("Kind", 1)
			hit := false
			So(func() {
				So(rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {
					hit = true
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("PutMulti", func() {
			nullCb := func(int, *Key, error) {}
			keys := []*Key{}
			vals := []PropertyMap{{}}
			So(rds.PutMulti(keys, vals, nullCb), ShouldErrLike, "mismatched keys/vals")
			So(rds.PutMulti(nil, nil, nullCb), ShouldBeNil)

			keys = append(keys, mkKey("aid", "ns", "Wut", 0, "Kind", 0))
			So(rds.PutMulti(keys, vals, nil), ShouldErrLike, "callback is nil")

			So(rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
				So(k, ShouldBeNil)
				So(IsErrInvalidKey(err), ShouldBeTrue)
			}), ShouldBeNil)

			keys = []*Key{mkKey("s~aid", "ns", "Kind", 0)}
			vals = []PropertyMap{nil}
			So(rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
				So(k, ShouldBeNil)
				So(err, ShouldErrLike, "nil vals entry")
			}), ShouldBeNil)

			vals = []PropertyMap{{}}
			hit := false
			So(func() {
				So(rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
					hit = true
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

		Convey("DeleteMulti", func() {
			So(rds.DeleteMulti(nil, func(int, error) {}), ShouldBeNil)

			So(rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, nil), ShouldErrLike, "is nil")
			So(rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, func(_ int, err error) {
				So(IsErrInvalidKey(err), ShouldBeTrue)
			}), ShouldBeNil)

			hit := false
			So(func() {
				So(rds.DeleteMulti([]*Key{mkKey("s~aid", "ns", "Kind", 1)}, func(int, error) {
					hit = true
				}), ShouldBeNil)
			}, ShouldPanic)
			So(hit, ShouldBeFalse)
		})

	})
}
