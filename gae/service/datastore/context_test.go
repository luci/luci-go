// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datastore

import (
	"strconv"
	"testing"

	"go.chromium.org/gae/service/info"

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

func (f fakeService) Constraints() Constraints { return Constraints{} }

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
				So(Raw(c), ShouldHaveSameTypeAs, &checkFilter{})
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
