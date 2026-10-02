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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestVM(t *testing.T) {
	t.Parallel()

	ftt.Run("VM", t, func(t *ftt.Test) {
		t.Run("SetZone", func(t *ftt.Test) {
			t.Run("no replacement", func(t *ftt.Test) {
				vm := &VM{
					Disk: []*Disk{
						{
							Type: "zones/zone/diskTypes/type",
						},
					},
					MachineType: "type",
				}
				vm.SetZone("zone")
				assert.Loosely(t, vm.Disk[0].Type, should.Equal("zones/zone/diskTypes/type"))
				assert.Loosely(t, vm.MachineType, should.Equal("type"))
				assert.Loosely(t, vm.Zone, should.Equal("zone"))
			})

			t.Run("replacement", func(t *ftt.Test) {
				t.Run("disk type", func(t *ftt.Test) {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "zones/{{.Zone}}/diskTypes/type",
							},
						},
						MachineType: "type",
					}
					vm.SetZone("zone")
					assert.Loosely(t, vm.Disk[0].Type, should.Equal("zones/zone/diskTypes/type"))
					assert.Loosely(t, vm.MachineType, should.Equal("type"))
					assert.Loosely(t, vm.Zone, should.Equal("zone"))
				})

				t.Run("machine type", func(t *ftt.Test) {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "zones/zone/diskTypes/type",
							},
						},
						MachineType: "{{.Zone}}/type",
					}
					vm.SetZone("zone")
					assert.Loosely(t, vm.Disk[0].Type, should.Equal("zones/zone/diskTypes/type"))
					assert.Loosely(t, vm.MachineType, should.Equal("zone/type"))
					assert.Loosely(t, vm.Zone, should.Equal("zone"))
				})

				t.Run("multiple", func(t *ftt.Test) {
					vm := &VM{
						Disk: []*Disk{
							{
								Type: "zones/{{.Zone}}/diskTypes/type-1",
							},
							{
								Type: "zones/{{.Zone}}/diskTypes/type-2",
							},
						},
						FallbackZones: []string{"us-central1-a"},
						MachineType:   "{{.Zone}}/type",
					}
					vm.SetZone("zone")
					assert.Loosely(t, vm.Disk[0].Type, should.Equal("zones/zone/diskTypes/type-1"))
					assert.Loosely(t, vm.Disk[1].Type, should.Equal("zones/zone/diskTypes/type-2"))
					assert.Loosely(t, vm.FallbackZones, should.BeNil)
					assert.Loosely(t, vm.MachineType, should.Equal("zone/type"))
					assert.Loosely(t, vm.Zone, should.Equal("zone"))
				})
			})
		})

		t.Run("Validate", func(t *ftt.Test) {
			c := &validation.Context{Context: context.Background()}

			t.Run("invalid", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					vm := &VM{}
					vm.Validate(c, true)
					err := c.Finalize().(*validation.Error).Errors
					assert.Loosely(t, err, should.UnwrapToErrStringLike("at least one disk is required"))
					assert.Loosely(t, err, should.UnwrapToErrStringLike("machine type is required"))
					assert.Loosely(t, err, should.UnwrapToErrStringLike("at least one network interface is required"))
					assert.Loosely(t, err, should.UnwrapToErrStringLike("project is required"))
					assert.Loosely(t, err, should.UnwrapToErrStringLike("zone is required"))
				})

				t.Run("metadata", func(t *ftt.Test) {
					t.Run("format", func(t *ftt.Test) {
						vm := &VM{
							Metadata: []*Metadata{
								{},
							},
						}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike("metadata from text must be in key:value form"))
					})

					t.Run("file", func(t *ftt.Test) {
						vm := &VM{
							Metadata: []*Metadata{
								{
									Metadata: &Metadata_FromFile{
										FromFile: "key:file",
									},
								},
							},
						}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike("metadata from text must be in key:value form"))
					})
				})

				t.Run("fallback zones", func(t *ftt.Test) {
					baseVM := func() *VM {
						return &VM{
							Disk: []*Disk{
								{
									Image: "global/images/image",
									Type:  "zones/{{.Zone}}/diskTypes/type",
								},
							},
							MachineType: "zones/{{.Zone}}/machineTypes/type",
							NetworkInterface: []*NetworkInterface{
								{},
							},
							Project: "project",
							Zone:    "us-central1-c",
						}
					}

					t.Run("too many", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{
							"us-central1-a",
							"us-central1-b",
							"us-central1-f",
							"us-central1-d",
						}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike("at most 3 fallback zones are allowed, got 4"))
					})

					t.Run("empty entry", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{""}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike("fallback zone 0 cannot be empty"))
					})

					t.Run("same as primary", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{"us-central1-c"}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike(`fallback zone "us-central1-c" cannot be the same as primary zone "us-central1-c"`))
					})

					t.Run("duplicate", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{"us-central1-a", "us-central1-a"}
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike(`duplicate fallback zone "us-central1-a"`))
					})

					t.Run("machine type missing template", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{"us-central1-a"}
						vm.MachineType = "zones/us-central1-c/machineTypes/type"
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike(`machine type "zones/us-central1-c/machineTypes/type" must contain {{.Zone}} template when fallback_zones is set`))
					})

					t.Run("disk type missing template", func(t *ftt.Test) {
						vm := baseVM()
						vm.FallbackZones = []string{"us-central1-a"}
						vm.Disk[0].Type = "zones/us-central1-c/diskTypes/type"
						vm.Validate(c, true)
						err := c.Finalize().(*validation.Error).Errors
						assert.Loosely(t, err, should.UnwrapToErrStringLike(`disk 0 type "zones/us-central1-c/diskTypes/type" must contain {{.Zone}} template when fallback_zones is set`))
					})
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				vm := &VM{
					Disk: []*Disk{
						{
							Image: "global/images/image",
							Type:  "zones/{{.Zone}}/diskTypes/type",
						},
					},
					MachineType: "zones/{{.Zone}}/machineTypes/type",
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
				vm.Validate(c, true)
				assert.Loosely(t, c.Finalize(), should.BeNil)

				t.Run("with fallback zones", func(t *ftt.Test) {
					vm.Zone = "us-central1-c"
					vm.FallbackZones = []string{"us-central1-a", "us-central1-b", "us-central1-f"}
					vm.Validate(c, true)
					assert.Loosely(t, c.Finalize(), should.BeNil)

					vm.SetZone("us-central1-a")
					vm.Validate(c, true)
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})
			})
		})
	})
}
