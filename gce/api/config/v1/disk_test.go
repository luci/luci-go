// Copyright 2019 The LUCI Authors.
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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDisk(t *testing.T) {
	t.Parallel()

	Convey("Disk", t, func() {
		Convey("isValidImage", func() {
			Convey("invalid", func() {
				So(isValidImage("image"), ShouldBeFalse)
				So(isValidImage("global/image"), ShouldBeFalse)
				So(isValidImage("projects/image"), ShouldBeFalse)
				So(isValidImage("projects/global/image"), ShouldBeFalse)
				So(isValidImage("projects/project/region/image"), ShouldBeFalse)
				So(isValidImage("projects/project/region/images/image"), ShouldBeFalse)
			})

			Convey("valid", func() {
				So(isValidImage("global/images/image"), ShouldBeTrue)
				So(isValidImage("projects/project/global/images/image"), ShouldBeTrue)
			})
		})

		Convey("GetImageBase", func() {
			Convey("short", func() {
				d := &Disk{
					Image: "global/images/image",
				}
				So(d.GetImageBase(), ShouldEqual, "image")
			})

			Convey("long", func() {
				d := &Disk{
					Image: "projects/project/global/images/image",
				}
				So(d.GetImageBase(), ShouldEqual, "image")
			})
		})

		Convey("Validate", func() {
			c := &validation.Context{Context: context.Background()}

			Convey("invalid", func() {
				d := &Disk{}
				d.Validate(c)
				err := c.Finalize().(*validation.Error).Errors
				So(err, ShouldContainErr, "image must match")
			})

			Convey("valid", func() {
				d := &Disk{
					Image: "global/images/image",
				}
				d.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})
		})
	})
}
