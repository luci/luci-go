// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/rawdatastore"
	"github.com/luci/luci-go/common/errors"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestBrokenFeatures(t *testing.T) {
	t.Parallel()

	e := errors.New("default err")

	cbe := func(expect string) func(rawdatastore.PropertyMap, error) {
		return func(_ rawdatastore.PropertyMap, err error) {
			So(err.Error(), ShouldContainSubstring, expect)
		}
	}

	cbn := func(rawdatastore.PropertyMap, error) {}

	Convey("BrokenFeatures", t, func() {
		c := memory.Use(context.Background())

		Convey("Can break rds", func() {
			Convey("without a default", func() {
				c, bf := FilterRDS(c, nil)
				rds := rawdatastore.Get(c)
				keys := []rawdatastore.Key{rds.NewKey("Wut", "", 1, nil)}

				Convey("by specifying an error", func() {
					bf.BreakFeatures(e, "GetMulti", "PutMulti")
					So(rds.GetMulti(keys, cbn), ShouldEqual, e)

					Convey("and you can unbreak them as well", func() {
						bf.UnbreakFeatures("GetMulti")

						err := rds.GetMulti(keys, cbe(rawdatastore.ErrNoSuchEntity.Error()))
						So(err, ShouldBeNil)

						Convey("no broken features at all is a shortcut", func() {
							bf.UnbreakFeatures("PutMulti")
							err := rds.GetMulti(keys, cbe(rawdatastore.ErrNoSuchEntity.Error()))
							So(err, ShouldBeNil)
						})
					})
				})

				Convey("Not specifying an error gets you a generic error", func() {
					bf.BreakFeatures(nil, "GetMulti")
					err := rds.GetMulti(keys, cbn)
					So(err.Error(), ShouldContainSubstring, `feature "GetMulti" is broken`)
				})
			})

			Convey("with a default", func() {
				c, bf := FilterRDS(c, e)
				rds := rawdatastore.Get(c)
				keys := []rawdatastore.Key{rds.NewKey("Wut", "", 1, nil)}
				bf.BreakFeatures(nil, "GetMulti")
				So(rds.GetMulti(keys, cbn), ShouldEqual, e)
			})
		})
	})
}
