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
			So(i.Disks, ShouldHaveLength, 0)
			So(i.Metadata.Items, ShouldHaveLength, 0)
			So(i.NetworkInterfaces, ShouldHaveLength, 0)
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
					Metadata: []*config.Metadata{
						{
							Metadata: &config.Metadata_FromText{
								FromText: "",
							},
						},
						{
							Metadata: &config.Metadata_FromText{
								FromText: "key1",
							},
						},
						{
							Metadata: &config.Metadata_FromText{
								FromText: "key2:",
							},
						},
						{
							Metadata: &config.Metadata_FromText{
								FromText: ":value3",
							},
						},
						{
							Metadata: &config.Metadata_FromText{
								FromText: "key4:value4",
							},
						},
						{
							Metadata: &config.Metadata_FromFile{
								FromFile: "key5:value5",
							},
						},
					},
					NetworkInterface: []*config.NetworkInterface{
						{
							AccessConfig: []*config.AccessConfig{
								{},
							},
							Network: "network",
						},
					},
				},
			}
			i := v.GetInstance()
			So(i.Disks, ShouldHaveLength, 1)
			So(i.Disks[0].AutoDelete, ShouldBeTrue)
			So(i.Disks[0].Boot, ShouldBeTrue)
			So(i.Disks[0].InitializeParams.DiskSizeGb, ShouldEqual, 100)
			So(i.Disks[0].InitializeParams.SourceImage, ShouldEqual, "image")
			So(i.MachineType, ShouldEqual, "type")
			So(i.Metadata.Items, ShouldHaveLength, 6)
			So(i.Metadata.Items[0].Key, ShouldEqual, "")
			So(i.Metadata.Items[0].Value, ShouldBeNil)
			So(i.Metadata.Items[1].Key, ShouldEqual, "key1")
			So(i.Metadata.Items[1].Value, ShouldBeNil)
			So(i.Metadata.Items[2].Key, ShouldEqual, "key2")
			So(i.Metadata.Items[2].Value, ShouldNotBeNil)
			So(i.Metadata.Items[3].Key, ShouldEqual, "")
			So(i.Metadata.Items[3].Value, ShouldNotBeNil)
			So(i.Metadata.Items[4].Key, ShouldEqual, "key4")
			So(i.Metadata.Items[4].Value, ShouldNotBeNil)
			So(i.Metadata.Items[5].Key, ShouldEqual, "")
			So(i.Metadata.Items[5].Value, ShouldBeNil)
			So(i.NetworkInterfaces, ShouldHaveLength, 1)
			So(i.NetworkInterfaces[0].AccessConfigs, ShouldHaveLength, 1)
			So(i.NetworkInterfaces[0].AccessConfigs[0].Type, ShouldEqual, "ONE_TO_ONE_NAT")
			So(i.NetworkInterfaces[0].Network, ShouldEqual, "network")
		})
	})
}
