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
	"fmt"
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

			invalidTestCases := []struct {
				name          string
				diskType string
				diskInterface DiskInterface
				image         string
				expected      string
			}{
				{"Persistent + NVMe", "pd-fake", DiskInterface_NVME, "", "persistent disk must use SCSI"},
				{"Persistent + No Image", "pd-fake", DiskInterface_SCSI, "", "image must match"},
				{"Scratch + Image", "local-ssd", DiskInterface_NVME, "global/images/image", "local ssd cannot use an image"},
			}
			for _, testCase := range invalidTestCases {
				Convey(fmt.Sprintf("invalid - %s", testCase.name), func() {
					d := &Disk{
						Type:  testCase.diskType,
						Interface: testCase.diskInterface,
						Image:     testCase.image,
					}
					d.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					So(err, ShouldContainErr, testCase.expected)
				})
			}

			validTestCases := []struct {
				name          string
				diskType      string
				diskInterface DiskInterface
				image         string
			}{
				{"Persistent + SCSI + Image", "pd-fake", DiskInterface_SCSI, "global/images/image"},
				{"Scratch + No Image", "local-ssd", DiskInterface_NVME, ""},
			}
			for _, testCase := range validTestCases {
				Convey(fmt.Sprintf("valid - %s", testCase.name), func() {
					d := &Disk{
						Type:  testCase.diskType,
						Interface: testCase.diskInterface,
						Image:     testCase.image,
					}
					d.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})
			}
		})
	})
}
