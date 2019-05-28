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

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Config", t, func() {
		srv := &Config{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		Convey("Delete", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Delete(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Delete(c, &config.DeleteRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				cfg, err := srv.Delete(c, &config.DeleteRequest{
					Id: "id",
				})
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &empty.Empty{})
				err = datastore.Get(c, &model.Config{
					ID: "id",
				})
				So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})

		Convey("Ensure", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Ensure(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("ID", func() {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Config: &config.Config{},
					})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				cfg, err := srv.Ensure(c, &config.EnsureRequest{
					Id: "id",
					Config: &config.Config{
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{},
							},
							MachineType: "type",
							NetworkInterface: []*config.NetworkInterface{
								{},
							},
							Project: "project",
							Zone:    "zone",
						},
						Lifetime: &config.TimePeriod{
							Time: &config.TimePeriod_Seconds{
								Seconds: 3600,
							},
						},
						Prefix: "prefix",
					},
				})
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &config.Config{
					Attributes: &config.VM{
						Disk: []*config.Disk{
							{},
						},
						MachineType: "type",
						Project:     "project",
						NetworkInterface: []*config.NetworkInterface{
							{},
						},
						Zone: "zone",
					},
					Lifetime: &config.TimePeriod{
						Time: &config.TimePeriod_Seconds{
							Seconds: 3600,
						},
					},
					Prefix: "prefix",
				})
			})
		})

		Convey("Get", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Get(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Get(c, &config.GetRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				Convey("not found", func() {
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					So(err, ShouldErrLike, "no config found")
					So(cfg, ShouldBeNil)
				})

				Convey("found", func() {
					datastore.Put(c, &model.Config{
						ID: "id",
						Config: config.Config{
							Prefix: "prefix",
						},
					})
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(cfg, ShouldResemble, &config.Config{
						Prefix: "prefix",
					})
				})
			})
		})

		Convey("List", func() {
			Convey("invalid", func() {
				Convey("page token", func() {
					req := &config.ListRequest{
						PageToken: "token",
					}
					_, err := srv.List(c, req)
					So(err, ShouldErrLike, "invalid page token")
				})
			})

			Convey("valid", func() {
				Convey("nil", func() {
					Convey("none", func() {
						rsp, err := srv.List(c, nil)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldBeEmpty)
					})

					Convey("one", func() {
						cfg := &model.Config{
							ID: "id",
						}
						So(datastore.Put(c, cfg), ShouldBeNil)

						rsp, err := srv.List(c, nil)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})
				})

				Convey("empty", func() {
					Convey("none", func() {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldBeEmpty)
					})

					Convey("one", func() {
						cfg := &model.Config{
							ID: "id",
						}
						So(datastore.Put(c, cfg), ShouldBeNil)

						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})
				})

				Convey("pages", func() {
					So(datastore.Put(c, &model.Config{ID: "id1"}), ShouldBeNil)
					So(datastore.Put(c, &model.Config{ID: "id2"}), ShouldBeNil)
					So(datastore.Put(c, &model.Config{ID: "id3"}), ShouldBeNil)

					Convey("default", func() {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldNotBeEmpty)
					})

					Convey("one", func() {
						req := &config.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})

					Convey("two", func() {
						req := &config.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 2)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})

					Convey("many", func() {
						req := &config.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 3)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldBeEmpty)
					})
				})
			})
		})
	})
}
