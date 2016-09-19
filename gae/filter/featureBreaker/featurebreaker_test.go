// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"

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
