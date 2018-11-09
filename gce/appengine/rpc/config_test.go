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

package rpc

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateBlock(t *testing.T) {
	t.Parallel()

	Convey("DeleteVMs", t, func() {
		c := memory.Use(context.Background())
		srv := &Config{}

		Convey("invalid", func() {
			Convey("nil", func() {
				v, err := srv.DeleteVMs(c, nil)
				So(err, ShouldErrLike, "request is required")
				So(v, ShouldBeNil)
			})

			Convey("empty", func() {
				v, err := srv.DeleteVMs(c, &config.DeleteVMsRequest{})
				So(err, ShouldErrLike, "ID is required")
				So(v, ShouldBeNil)
			})
		})

		Convey("valid", func() {
			datastore.Put(c, &model.VMs{
				ID: "id",
				Config: config.Block{
					Amount: 1,
					Attributes: &config.VM{
						Project: "project",
					},
					Prefix: "prefix",
				},
			})
			v, err := srv.DeleteVMs(c, &config.DeleteVMsRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &empty.Empty{})
			err = datastore.Get(c, &model.VMs{
				ID: "id",
			})
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})

	Convey("EnsureVMs", t, func() {
		c := memory.Use(context.Background())
		srv := &Config{}

		Convey("invalid", func() {
			Convey("nil", func() {
				v, err := srv.EnsureVMs(c, nil)
				So(err, ShouldErrLike, "request is required")
				So(v, ShouldBeNil)
			})

			Convey("empty", func() {
				v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{})
				So(err, ShouldErrLike, "ID is required")
				So(v, ShouldBeNil)
			})

			Convey("ID missing", func() {
				v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{
					Vms: &config.Block{},
				})
				So(err, ShouldErrLike, "ID is required")
				So(v, ShouldBeNil)
			})

			Convey("block missing", func() {
				v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{
					Id: "id",
				})
				So(err, ShouldErrLike, "VMs block is required")
				So(v, ShouldBeNil)
			})
		})

		Convey("valid", func() {
			v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{
				Id: "id",
				Vms: &config.Block{
					Attributes: &config.VM{
						Project: "project",
					},
					Prefix: "prefix",
				},
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &config.Block{
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
		})
	})

	Convey("GetVMs", t, func() {
		c := memory.Use(context.Background())
		srv := &Config{}

		Convey("invalid", func() {
			Convey("nil", func() {
				v, err := srv.GetVMs(c, nil)
				So(err, ShouldErrLike, "request is required")
				So(v, ShouldBeNil)
			})

			Convey("empty", func() {
				v, err := srv.GetVMs(c, &config.GetVMsRequest{})
				So(err, ShouldErrLike, "ID is required")
				So(v, ShouldBeNil)
			})
		})

		Convey("valid", func() {
			Convey("not found", func() {
				v, err := srv.GetVMs(c, &config.GetVMsRequest{
					Id: "id",
				})
				So(err, ShouldErrLike, "no VMs block found")
				So(v, ShouldBeNil)
			})

			Convey("found", func() {
				datastore.Put(c, &model.VMs{
					ID: "id",
					Config: config.Block{
						Amount: 1,
						Attributes: &config.VM{
							Project: "project",
						},
						Prefix: "prefix",
					},
				})
				v, err := srv.GetVMs(c, &config.GetVMsRequest{
					Id: "id",
				})
				So(err, ShouldBeNil)
				So(v, ShouldResemble, &config.Block{
					Amount: 1,
					Attributes: &config.VM{
						Project: "project",
					},
					Prefix: "prefix",
				})
			})
		})
	})
}
