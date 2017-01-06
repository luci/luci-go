// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"strconv"
	"testing"

	"github.com/luci/gae/service/info"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeInfo struct{ info.RawInterface }

func (fakeInfo) GetNamespace() string        { return "ns" }
func (fakeInfo) AppID() string               { return "aid" }
func (fakeInfo) FullyQualifiedAppID() string { return "s~aid" }

type fakeService struct{ RawInterface }

type fakeFilt struct{ RawInterface }

func (f fakeService) DecodeCursor(s string) (Cursor, error) {
	v, err := strconv.Atoi(s)
	return fakeCursor(v), err
}

type fakeCursor int

func (f fakeCursor) String() string {
	return strconv.Itoa(int(f))
}

func TestServices(t *testing.T) {
	t.Parallel()

	Convey("Test service interfaces", t, func() {
		c := context.Background()
		Convey("without adding anything", func() {
			So(Raw(c), ShouldBeNil)
		})

		Convey("adding a basic implementation", func() {
			c = SetRaw(info.Set(c, fakeInfo{}), fakeService{})

			Convey("lets you pull them back out", func() {
				So(Raw(c), ShouldResemble, &checkFilter{
					RawInterface: fakeService{},
					kc:           MkKeyContext("s~aid", "ns"),
				})
			})

			Convey("and lets you add filters", func() {
				c = AddRawFilters(c, func(ic context.Context, rds RawInterface) RawInterface {
					return fakeFilt{rds}
				})

				curs, err := DecodeCursor(c, "123")
				So(err, ShouldBeNil)
				So(curs.String(), ShouldEqual, "123")
			})
		})
		Convey("adding zero filters does nothing", func() {
			So(AddRawFilters(c), ShouldEqual, c)
		})
	})
}
