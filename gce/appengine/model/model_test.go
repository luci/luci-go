// Copyright 2018 The LUCI Authors.
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

package model

import (
	"context"
	"testing"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVMs(t *testing.T) {
	t.Parallel()

	Convey("VMs", t, func() {
		c := memory.Use(context.Background())
		v := &VMs{ID: "id"}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VMs{
			ID: "id",
			Config: config.Block{
				Attributes: &config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
						},
					},
					Project: "project",
				},
				Prefix: "prefix",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, v)
		So(err, ShouldBeNil)
		So(v, ShouldResemble, &VMs{
			ID: "id",
			Config: config.Block{
				Attributes: &config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
						},
					},
					Project: "project",
				},
				Prefix: "prefix",
			},
		})
	})
}

func TestVM(t *testing.T) {
	t.Parallel()

	Convey("VM", t, func() {
		c := memory.Use(context.Background())
		v := &VM{ID: "id"}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, v)
		So(err, ShouldBeNil)
		So(v, ShouldResemble, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		})
	})

	Convey("GetInstance", t, func() {
		Convey("empty", func() {
			v := &VM{}
			i := v.GetInstance()
			So(i, ShouldResemble, &compute.Instance{
				Disks: []*compute.AttachedDisk{},
				NetworkInterfaces: []*compute.NetworkInterface{
					{},
				},
			})
		})

		Convey("non-empty", func() {
			v := &VM{
				Attributes: config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
							Size:  100,
						},
					},
					MachineType: "type",
				},
			}
			i := v.GetInstance()
			So(i, ShouldResemble, &compute.Instance{
				Disks: []*compute.AttachedDisk{
					{
						AutoDelete: true,
						Boot:       true,
						InitializeParams: &compute.AttachedDiskInitializeParams{
							DiskSizeGb:  100,
							SourceImage: "image",
						},
					},
				},
				MachineType: "type",
				NetworkInterfaces: []*compute.NetworkInterface{
					{},
				},
			})
		})
	})
}
