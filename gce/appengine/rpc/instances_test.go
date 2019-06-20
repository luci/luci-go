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

package rpc

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInstances(t *testing.T) {
	t.Parallel()

	Convey("Instances", t, func() {
		srv := &Instances{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		Convey("List", func() {
			Convey("invalid", func() {
				Convey("filter", func() {
					req := &instances.ListRequest{
						Filter: "filter",
					}
					_, err := srv.List(c, req)
					So(err, ShouldErrLike, "invalid filter expression")
				})

				Convey("page token", func() {
					req := &instances.ListRequest{
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
						So(rsp.Instances, ShouldBeEmpty)
					})

					Convey("one", func() {
						vm := &model.VM{
							ID: "id",
						}
						So(datastore.Put(c, vm), ShouldBeNil)

						rsp, err := srv.List(c, nil)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.Instances[0].Id, ShouldEqual, "id")
					})
				})

				Convey("empty", func() {
					Convey("none", func() {
						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldBeEmpty)
					})

					Convey("one", func() {
						vm := &model.VM{
							ID: "id",
						}
						So(datastore.Put(c, vm), ShouldBeNil)

						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.Instances[0].Id, ShouldEqual, "id")
					})
				})

				Convey("pages", func() {
					So(datastore.Put(c, &model.VM{ID: "id1"}), ShouldBeNil)
					So(datastore.Put(c, &model.VM{ID: "id2"}), ShouldBeNil)
					So(datastore.Put(c, &model.VM{ID: "id3"}), ShouldBeNil)

					Convey("default", func() {
						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldNotBeEmpty)
					})

					Convey("one", func() {
						req := &instances.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.Instances[0].Id, ShouldEqual, "id1")
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.Instances[0].Id, ShouldEqual, "id2")
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.Instances[0].Id, ShouldEqual, "id3")
						So(rsp.NextPageToken, ShouldBeEmpty)
					})

					Convey("two", func() {
						req := &instances.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 2)
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 1)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})

					Convey("many", func() {
						req := &instances.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Instances, ShouldHaveLength, 3)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})
				})

				Convey("filter", func() {
					vm := &model.VM{
						ID: "id",
						AttributesIndexed: []string{
							"disk.image:image2",
						},
					}
					So(datastore.Put(c, vm), ShouldBeNil)

					req := &instances.ListRequest{
						Filter: "instances.disks.image=image1",
					}
					rsp, err := srv.List(c, req)
					So(err, ShouldBeNil)
					So(rsp.Instances, ShouldBeEmpty)

					req.Filter = "instances.disks.image=image2"
					rsp, err = srv.List(c, req)
					So(err, ShouldBeNil)
					So(rsp.Instances, ShouldHaveLength, 1)
				})

				Convey("prefix", func() {
					vm := &model.VM{
						ID:     "id1",
						Prefix: "prefix1",
					}
					So(datastore.Put(c, vm), ShouldBeNil)
					vm = &model.VM{
						ID:     "id2",
						Prefix: "prefix2",
					}
					So(datastore.Put(c, vm), ShouldBeNil)

					req := &instances.ListRequest{
						Prefix: "prefix1",
					}
					rsp, err := srv.List(c, req)
					So(err, ShouldBeNil)
					So(rsp.Instances, ShouldHaveLength, 1)
					So(rsp.Instances[0].Id, ShouldEqual, "id1")
				})
			})
		})
	})
}
