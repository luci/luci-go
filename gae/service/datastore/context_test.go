// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"testing"

	"github.com/luci/gae/service/info"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type fakeInfo struct{ info.Interface }

func (fakeInfo) GetNamespace() string        { return "ns" }
func (fakeInfo) AppID() string               { return "aid" }
func (fakeInfo) FullyQualifiedAppID() string { return "s~aid" }

type fakeService struct{ RawInterface }

type fakeFilt struct{ RawInterface }

func (f fakeService) DecodeCursor(s string) (Cursor, error) {
	return fakeCursor(s), nil
}

type fakeCursor string

func (f fakeCursor) String() string {
	return string(f)
}

func TestServices(t *testing.T) {
	t.Parallel()

	Convey("Test service interfaces", t, func() {
		c := context.Background()
		Convey("without adding anything", func() {
			So(GetRaw(c), ShouldBeNil)
		})

		Convey("adding a basic implementation", func() {
			c = SetRaw(info.Set(c, fakeInfo{}), fakeService{})

			Convey("lets you pull them back out", func() {
				So(GetRaw(c), ShouldResemble, &checkFilter{fakeService{}, "s~aid", "ns"})
			})

			Convey("and lets you add filters", func() {
				c = AddRawFilters(c, func(ic context.Context, rds RawInterface) RawInterface {
					return fakeFilt{rds}
				})

				curs, err := Get(c).DecodeCursor("pants")
				So(err, ShouldBeNil)
				So(curs.String(), ShouldEqual, "pants")
			})
		})
		Convey("adding zero filters does nothing", func() {
			So(AddRawFilters(c), ShouldEqual, c)
		})
	})
}
