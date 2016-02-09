// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"math"
	"testing"

	"github.com/luci/gae/service/datastore"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestUInt32(t *testing.T) {
	t.Parallel()

	Convey("Uint32", t, func() {
		u := UInt32(17)

		Convey("ToProperty", func() {
			p, err := u.ToProperty()
			So(err, ShouldBeNil)
			So(p, ShouldResemble, datastore.MkPropertyNI(17))
		})

		Convey("FromProperty", func() {
			Convey("bad type", func() {
				p := datastore.MkProperty("hi")
				So(u.FromProperty(p), ShouldErrLike, "unable to project PTString to PTInt")

				p = datastore.MkProperty(-110)
				So(u.FromProperty(p), ShouldErrLike, "out of bounds")

				p = datastore.MkProperty(int64(math.MaxUint32 + 1))
				So(u.FromProperty(p), ShouldErrLike, "out of bounds")

				p = datastore.MkProperty(1394)
				So(u.FromProperty(p), ShouldBeNil)
				So(uint32(u), ShouldResemble, uint32(1394))
			})
		})
	})
}
