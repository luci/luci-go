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

package memory

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/gce/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("DeleteVMs", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("nil", func() {
			v, err := srv.DeleteVMs(c, nil)
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &empty.Empty{})
		})

		Convey("empty", func() {
			v, err := srv.DeleteVMs(c, &config.DeleteVMsRequest{})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &empty.Empty{})
		})

		Convey("ID", func() {
			v, err := srv.DeleteVMs(c, &config.DeleteVMsRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &empty.Empty{})
		})

		Convey("deleted", func() {
			srv.vms.Store("id", &config.Block{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
			v, err := srv.DeleteVMs(c, &config.DeleteVMsRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &empty.Empty{})
			_, ok := srv.vms.Load("id")
			So(ok, ShouldBeFalse)
		})
	})

	Convey("EnsureVMs", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("nil", func() {
			v, err := srv.EnsureVMs(c, nil)
			So(err, ShouldBeNil)
			So(v, ShouldResemble, (*config.Block)(nil))
			_, ok := srv.vms.Load("")
			So(ok, ShouldBeTrue)
		})

		Convey("empty", func() {
			v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, (*config.Block)(nil))
			_, ok := srv.vms.Load("")
			So(ok, ShouldBeTrue)
		})

		Convey("ID", func() {
			v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{
				Id: "id",
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, (*config.Block)(nil))
			_, ok := srv.vms.Load("id")
			So(ok, ShouldBeTrue)
		})

		Convey("VMs", func() {
			v, err := srv.EnsureVMs(c, &config.EnsureVMsRequest{
				Id: "id",
				Vms: &config.Block{
					Amount: 1,
					Attributes: &config.VM{
						Project: "project",
					},
					Prefix: "prefix",
				},
			})
			So(err, ShouldBeNil)
			So(v, ShouldResemble, &config.Block{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			})
			_, ok := srv.vms.Load("id")
			So(ok, ShouldBeTrue)
		})
	})

	Convey("GetVMs", t, func() {
		c := context.Background()
		srv := &Config{}

		Convey("not found", func() {
			Convey("nil", func() {
				v, err := srv.GetVMs(c, nil)
				So(err, ShouldErrLike, "no VMs block found")
				So(v, ShouldBeNil)
			})

			Convey("empty", func() {
				v, err := srv.GetVMs(c, &config.GetVMsRequest{})
				So(err, ShouldErrLike, "no VMs block found")
				So(v, ShouldBeNil)
			})

			Convey("ID", func() {
				v, err := srv.GetVMs(c, &config.GetVMsRequest{
					Id: "id",
				})
				So(err, ShouldErrLike, "no VMs block found")
				So(v, ShouldBeNil)
			})
		})

		Convey("found", func() {
			srv.vms.Store("id", &config.Block{
				Amount: 1,
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
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
}
