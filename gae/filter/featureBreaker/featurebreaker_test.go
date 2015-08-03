// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestBrokenFeatures(t *testing.T) {
	t.Parallel()

	e := errors.New("default err")

	Convey("BrokenFeatures", t, func() {
		c := memory.Use(context.Background())

		Convey("Can break ds", func() {
			Convey("without a default", func() {
				c, bf := FilterRDS(c, nil)
				ds := datastore.Get(c)
				vals := []datastore.PropertyMap{{
					"$key": {datastore.MkPropertyNI(ds.NewKey("Wut", "", 1, nil))},
				}}

				Convey("by specifying an error", func() {
					bf.BreakFeatures(e, "GetMulti", "PutMulti")
					So(ds.GetMulti(vals), ShouldEqual, e)

					Convey("and you can unbreak them as well", func() {
						bf.UnbreakFeatures("GetMulti")

						So(errors.SingleError(ds.GetMulti(vals)), ShouldEqual, datastore.ErrNoSuchEntity)

						Convey("no broken features at all is a shortcut", func() {
							bf.UnbreakFeatures("PutMulti")
							So(errors.SingleError(ds.GetMulti(vals)), ShouldEqual, datastore.ErrNoSuchEntity)
						})
					})
				})

				Convey("Not specifying an error gets you a generic error", func() {
					bf.BreakFeatures(nil, "GetMulti")
					So(ds.GetMulti(vals).Error(), ShouldContainSubstring, `feature "GetMulti" is broken`)
				})
			})

			Convey("with a default", func() {
				c, bf := FilterRDS(c, e)
				ds := datastore.Get(c)
				vals := []datastore.PropertyMap{{
					"$key": {datastore.MkPropertyNI(ds.NewKey("Wut", "", 1, nil))},
				}}
				bf.BreakFeatures(nil, "GetMulti")
				So(ds.GetMulti(vals), ShouldEqual, e)
			})
		})
	})
}
