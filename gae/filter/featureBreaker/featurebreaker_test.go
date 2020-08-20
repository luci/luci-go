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

package featureBreaker

import (
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBrokenFeatures(t *testing.T) {
	t.Parallel()

	e := errors.New("default err")

	Convey("BrokenFeatures", t, func() {
		c := memory.Use(context.Background())

		Convey("Can break ds", func() {
			Convey("without a default", func() {
				c, bf := FilterRDS(c, nil)
				vals := []ds.PropertyMap{{
					"$key": ds.MkPropertyNI(ds.NewKey(c, "Wut", "", 1, nil)),
				}}

				Convey("by specifying an error", func() {
					bf.BreakFeatures(e, "GetMulti", "PutMulti")
					So(ds.Get(c, vals), ShouldEqual, e)

					Convey("and you can unbreak them as well", func() {
						bf.UnbreakFeatures("GetMulti")

						So(errors.SingleError(ds.Get(c, vals)), ShouldEqual, ds.ErrNoSuchEntity)

						Convey("no broken features at all is a shortcut", func() {
							bf.UnbreakFeatures("PutMulti")
							So(errors.SingleError(ds.Get(c, vals)), ShouldEqual, ds.ErrNoSuchEntity)
						})
					})
				})

				Convey("Not specifying an error gets you a generic error", func() {
					bf.BreakFeatures(nil, "GetMulti")
					So(ds.Get(c, vals).Error(), ShouldContainSubstring, `feature "GetMulti" is broken`)
				})

				Convey("Callback work and receives correct context", func() {
					errToReturn := errors.New("err from callback")
					key := "some key"

					bf.BreakFeaturesWithCallback(func(c context.Context, feature string) error {
						So(c.Value(&key), ShouldEqual, "some value")
						return errToReturn
					}, "GetMulti")

					ctx := context.WithValue(c, &key, "some value")
					So(ds.Get(ctx, vals), ShouldEqual, errToReturn)

					errToReturn = nil
					So(errors.SingleError(ds.Get(ctx, vals)), ShouldEqual, ds.ErrNoSuchEntity)
				})

				Convey("Transaction hooks work", func() {
					// A sequence of errors emulating a bunch of failing RPCs that cause
					// the transaction body to be retried once.
					errs := []struct {
						name string
						err  error
					}{
						{"BeginTransaction", nil},
						{"CommitTransaction", ds.ErrConcurrentTransaction},
						{"BeginTransaction", ds.ErrConcurrentTransaction},
						{"BeginTransaction", nil},
						{"CommitTransaction", nil},
					}

					bf.BreakFeaturesWithCallback(func(c context.Context, feature string) error {
						So(len(errs), ShouldBeGreaterThan, 0)
						So(errs[0].name, ShouldEqual, feature)
						err := errs[0].err
						errs = errs[1:]
						return err
					}, "BeginTransaction", "CommitTransaction")

					calls := 0
					So(ds.RunInTransaction(c, func(c context.Context) error {
						calls++
						return nil
					}, nil), ShouldBeNil)

					So(calls, ShouldEqual, 2)
					So(errs, ShouldBeEmpty)
				})
			})

			Convey("with a default", func() {
				c, bf := FilterRDS(c, e)
				vals := []ds.PropertyMap{{
					"$key": ds.MkPropertyNI(ds.NewKey(c, "Wut", "", 1, nil)),
				}}
				bf.BreakFeatures(nil, "GetMulti")
				So(ds.Get(c, vals), ShouldEqual, e)
			})
		})
	})
}
