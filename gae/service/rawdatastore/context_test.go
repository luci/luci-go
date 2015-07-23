// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type fakeService struct{ Interface }

type fakeFilt struct{ Interface }

func (fakeFilt) Count(Query) (int, error) {
	return 0, errors.New("wow")
}

func TestServices(t *testing.T) {
	t.Parallel()

	Convey("Test service interfaces", t, func() {
		c := context.Background()
		Convey("without adding anything", func() {
			So(Get(c), ShouldBeNil)
		})

		Convey("adding a basic implementation", func() {
			c = Set(c, fakeService{})

			Convey("lets you pull them back out", func() {
				So(Get(c), ShouldResemble, fakeService{})
			})

			Convey("and lets you add filters", func() {
				c = AddFilters(c, func(ic context.Context, rds Interface) Interface {
					return fakeFilt{rds}
				})

				_, err := Get(c).Count(nil)
				So(err.Error(), ShouldEqual, "wow")
			})
		})
		Convey("adding zero filters does nothing", func() {
			So(AddFilters(c), ShouldEqual, c)
		})
	})
}
