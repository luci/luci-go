// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"errors"
	"golang.org/x/net/context"
	"testing"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/memory"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBrokenFeatures(t *testing.T) {
	t.Parallel()

	e := errors.New("default err")

	Convey("BrokenFeatures", t, func() {
		c := memory.Use(context.Background())

		Convey("Can break rds", func() {
			Convey("without a default", func() {
				c, bf := FilterRDS(c, nil)
				rds := gae.GetRDS(c)

				Convey("by specifying an error", func() {
					bf.BreakFeatures(e, "Get", "Put")
					So(rds.Get(nil, nil), ShouldEqual, e)

					Convey("and you can unbreak them as well", func() {
						bf.UnbreakFeatures("Get")
						So(rds.Get(nil, nil), ShouldEqual, gae.ErrDSInvalidKey)

						Convey("no broken features at all is a shortcut", func() {
							bf.UnbreakFeatures("Put")
							So(rds.Get(nil, nil), ShouldEqual, gae.ErrDSInvalidKey)
						})
					})
				})

				Convey("Not specifying an error gets you a generic error", func() {
					bf.BreakFeatures(nil, "Get")
					So(rds.Get(nil, nil).Error(), ShouldEqual, `feature "Get" is broken`)
				})
			})

			Convey("with a default", func() {
				c, bf := FilterRDS(c, e)
				rds := gae.GetRDS(c)
				bf.BreakFeatures(nil, "Get")
				So(rds.Get(nil, nil), ShouldEqual, e)
			})
		})
	})
}
