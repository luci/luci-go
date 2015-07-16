// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"errors"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeRDS struct{ RawDatastore }
type fakeMC struct{ Memcache }
type fakeTQ struct{ TaskQueue }
type fakeGI struct{ GlobalInfo }

type fakeFiltRDS struct{ RawDatastore }

func (fakeFiltRDS) Count(DSQuery) (int, error) {
	return 0, errors.New("wow")
}

type fakeFiltTQ struct{ TaskQueue }

func (fakeFiltTQ) Purge(string) error {
	return errors.New("wow")
}

type fakeFiltMC struct{ Memcache }

func (fakeFiltMC) Flush() error {
	return errors.New("wow")
}

type fakeFiltGI struct{ GlobalInfo }

func (fakeFiltGI) Namespace(string) (context.Context, error) {
	return nil, errors.New("wow")
}

func TestServices(t *testing.T) {
	t.Parallel()

	Convey("Test service interfaces", t, func() {
		c := context.Background()
		Convey("without adding anything", func() {
			So(GetRDS(c), ShouldBeNil)
			So(GetTQ(c), ShouldBeNil)
			So(GetMC(c), ShouldBeNil)
			So(GetGI(c), ShouldBeNil)
		})

		Convey("adding a basic implementation", func() {
			c = SetRDS(c, fakeRDS{})
			c = SetTQ(c, fakeTQ{})
			c = SetMC(c, fakeMC{})
			c = SetGI(c, fakeGI{})

			Convey("lets you pull them back out", func() {
				So(GetRDS(c), ShouldResemble, fakeRDS{})
				So(GetTQ(c), ShouldResemble, fakeTQ{})
				So(GetMC(c), ShouldResemble, fakeMC{})
				So(GetGI(c), ShouldResemble, fakeGI{})
			})

			Convey("and lets you add filters", func() {
				c = AddRDSFilters(c, func(ic context.Context, rds RawDatastore) RawDatastore {
					return fakeFiltRDS{rds}
				})
				c = AddTQFilters(c, func(ic context.Context, tq TaskQueue) TaskQueue {
					return fakeFiltTQ{tq}
				})
				c = AddMCFilters(c, func(ic context.Context, mc Memcache) Memcache {
					return fakeFiltMC{mc}
				})
				c = AddGIFilters(c, func(ic context.Context, gi GlobalInfo) GlobalInfo {
					return fakeFiltGI{gi}
				})

				_, err := GetRDS(c).Count(nil)
				So(err.Error(), ShouldEqual, "wow")

				So(GetTQ(c).Purge("").Error(), ShouldEqual, "wow")

				So(GetMC(c).Flush().Error(), ShouldEqual, "wow")

				_, err = GetGI(c).Namespace("")
				So(err.Error(), ShouldEqual, "wow")
			})
		})
		Convey("adding zero filters does nothing", func() {
			So(AddRDSFilters(c), ShouldEqual, c)
			So(AddTQFilters(c), ShouldEqual, c)
			So(AddMCFilters(c), ShouldEqual, c)
			So(AddGIFilters(c), ShouldEqual, c)
		})
	})
}
