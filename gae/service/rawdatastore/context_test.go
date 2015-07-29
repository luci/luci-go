// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"testing"

	"github.com/luci/gae/service/info"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type fakeInfo struct{ info.Interface }

func (fakeInfo) GetNamespace() string { return "ns" }
func (fakeInfo) AppID() string        { return "aid" }

type fakeService struct{ Interface }

type fakeFilt struct{ Interface }

func (fakeFilt) NewKey(kind, stringID string, intID int64, parent Key) Key {
	return NewKey("aid", "ns", kind, stringID, intID, parent)
}

func TestServices(t *testing.T) {
	t.Parallel()

	Convey("Test service interfaces", t, func() {
		c := context.Background()
		Convey("without adding anything", func() {
			So(Get(c), ShouldBeNil)
		})

		Convey("adding a basic implementation", func() {
			c = Set(info.Set(c, fakeInfo{}), fakeService{})

			Convey("lets you pull them back out", func() {
				So(Get(c), ShouldResemble, &checkFilter{fakeService{}, "aid", "ns"})
			})

			Convey("and lets you add filters", func() {
				c = AddFilters(c, func(ic context.Context, rds Interface) Interface {
					return fakeFilt{rds}
				})

				k := Get(c).NewKey("Kind", "", 1, nil)
				So(KeysEqual(k, NewKey("aid", "ns", "Kind", "", 1, nil)), ShouldBeTrue)
			})
		})
		Convey("adding zero filters does nothing", func() {
			So(AddFilters(c), ShouldEqual, c)
		})
	})
}
