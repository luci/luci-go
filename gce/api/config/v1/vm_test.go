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

func TestValidateVM(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				vm := &VM{}
				vm.Validate(c)
				err := c.Finalize().(*validation.Error).Errors
				So(err, ShouldContainErr, "at least one disk is required")
				So(err, ShouldContainErr, "machine type is required")
				So(err, ShouldContainErr, "at least one network interface is required")
				So(err, ShouldContainErr, "project is required")
				So(err, ShouldContainErr, "zone is required")
			})

			Convey("metadata", func() {
				Convey("format", func() {
					vm := &VM{
						Metadata: []*Metadata{
							{},
						},
					}
					vm.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					So(err, ShouldContainErr, "metadata from text must be in key:value form")
				})

				Convey("file", func() {
					vm := &VM{
						Metadata: []*Metadata{
							{
								Metadata: &Metadata_FromFile{
									FromFile: "key:file",
								},
							},
						},
					}
					vm.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					So(err, ShouldContainErr, "metadata from text must be in key:value form")
				})
			})
		})

		Convey("valid", func() {
			vm := &VM{
				Disk: []*Disk{
					{},
				},
				MachineType: "type",
				Metadata: []*Metadata{
					{
						Metadata: &Metadata_FromText{
							FromText: "key:value",
						},
					},
				},
				NetworkInterface: []*NetworkInterface{
					{},
				},
				Project: "project",
				Zone:    "zone",
			}
			vm.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})
	})
}
