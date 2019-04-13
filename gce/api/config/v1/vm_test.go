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

func TestVM(t *testing.T) {
	t.Parallel()

	Convey("VM", t, func() {
		Convey("SetZone", func() {
			Convey("text", func() {
				vm := &VM{
					Disk: []*Disk{
						{
							Type: "type",
						},
					},
					MachineType: "type",
				}
				So(vm.SetZone("zone"), ShouldBeNil)
				So(vm.Disk[0].Type, ShouldEqual, "type")
				So(vm.MachineType, ShouldEqual, "type")
				So(vm.Zone, ShouldEqual, "zone")
			})

			Convey("template", func() {
				Convey("disk type", func() {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "{{.Zone}}/type",
							},
						},
						MachineType: "{{.Zone}}/{{}}",
					}
					So(vm.SetZone("zone"), ShouldErrLike, "failed to substitute")
					So(vm.Disk[0].Type, ShouldEqual, "{{.Zone}}/type")
					So(vm.MachineType, ShouldEqual, "{{.Zone}}/{{}}")
					So(vm.Zone, ShouldBeEmpty)

					vm.MachineType = ""
					So(vm.SetZone("zone"), ShouldBeNil)
					So(vm.Disk[0].Type, ShouldEqual, "zone/type")
					So(vm.MachineType, ShouldBeEmpty)
					So(vm.Zone, ShouldEqual, "zone")
				})

				Convey("machine type", func() {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "{{.Zone}}/{{}}",
							},
						},
						MachineType: "{{.Zone}}/type",
					}
					So(vm.SetZone("zone"), ShouldErrLike, "failed to substitute")
					So(vm.Disk[0].Type, ShouldEqual, "{{.Zone}}/{{}}")
					So(vm.MachineType, ShouldEqual, "{{.Zone}}/type")
					So(vm.Zone, ShouldBeEmpty)

					vm.Disk[0].Type = ""
					So(vm.SetZone("zone"), ShouldBeNil)
					So(vm.Disk[0].Type, ShouldBeEmpty)
					So(vm.MachineType, ShouldEqual, "zone/type")
					So(vm.Zone, ShouldEqual, "zone")
				})
			})
		})

		Convey("substituteZone", func() {
			Convey("invalid", func() {
				_, err := substituteZone("{{.Zone}}/{{}}", "zone")
				So(err, ShouldNotBeNil)
			})

			Convey("valid", func() {
				Convey("empty", func() {
					s, err := substituteZone("", "zone")
					So(err, ShouldBeNil)
					So(s, ShouldBeEmpty)
				})

				Convey("text", func() {
					s, err := substituteZone("zone-1", "zone-2")
					So(err, ShouldBeNil)
					So(s, ShouldEqual, "zone-1")
				})

				Convey("template", func() {
					s, err := substituteZone("{{.Zone}}", "zone")
					So(err, ShouldBeNil)
					So(s, ShouldEqual, "zone")
				})
			})
		})

		Convey("Validate", func() {
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

				Convey("disk type", func() {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "{{.Zone}}/{{}}",
							},
						},
					}
					vm.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					So(err, ShouldContainErr, "template")
				})

				Convey("machine type", func() {
					vm := &VM{
						MachineType: "{{.Zone}}/{{}}",
					}
					vm.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					So(err, ShouldContainErr, "template")
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
						{
							Type: "{{.Zone}}/type",
						},
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
	})
}
