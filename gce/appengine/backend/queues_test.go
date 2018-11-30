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

package backend

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueues(t *testing.T) {
	t.Parallel()

	Convey("queues", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withCompute(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), gce)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("create", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := create(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
				})

				Convey("empty", func() {
					err := create(c, &tasks.Create{})
					So(err, ShouldErrLike, "ID is required")
				})

				Convey("missing", func() {
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to fetch VM")
				})
			})

			Convey("valid", func() {
				Convey("exists", func() {
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
						URL:      "url",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldBeNil)
				})

				Convey("creates", func() {
					Convey("names", func() {
						rt.Handler = func(req interface{}) interface{} {
							inst, ok := req.(*compute.Instance)
							So(ok, ShouldBeTrue)
							So(inst.Name, ShouldNotBeEmpty)
							So(inst.NetworkInterfaces, ShouldHaveLength, 1)
							return &compute.Operation{}
						}
						rt.Type = reflect.TypeOf(compute.Instance{})
						datastore.Put(c, &model.VM{
							ID: "id",
						})
						err := create(c, &tasks.Create{
							Id: "id",
						})
						So(err, ShouldBeNil)
					})

					Convey("named", func() {
						rt.Handler = func(req interface{}) interface{} {
							inst, ok := req.(*compute.Instance)
							So(ok, ShouldBeTrue)
							So(inst.Name, ShouldEqual, "name")
							So(inst.NetworkInterfaces, ShouldHaveLength, 1)
							return &compute.Operation{}
						}
						rt.Type = reflect.TypeOf(compute.Instance{})
						datastore.Put(c, &model.VM{
							ID:       "id",
							Hostname: "name",
						})
						err := create(c, &tasks.Create{
							Id: "id",
						})
						So(err, ShouldBeNil)
					})
				})
			})
		})

		Convey("ensure", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := ensure(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
				})

				Convey("empty", func() {
					err := ensure(c, &tasks.Ensure{})
					So(err, ShouldErrLike, "ID is required")
				})
			})

			Convey("valid", func() {
				Convey("nil", func() {
					err := ensure(c, &tasks.Ensure{
						Id: "id",
					})
					So(err, ShouldBeNil)
					err = datastore.Get(c, &model.VM{
						ID: "id",
					})
					So(err, ShouldBeNil)
				})

				Convey("empty", func() {
					err := ensure(c, &tasks.Ensure{
						Id:         "id",
						Attributes: &config.VM{},
					})
					So(err, ShouldBeNil)
					err = datastore.Get(c, &model.VM{
						ID: "id",
					})
					So(err, ShouldBeNil)
				})

				Convey("non-empty", func() {
					err := ensure(c, &tasks.Ensure{
						Id: "id",
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					err = datastore.Get(c, v)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
					})
				})

				Convey("updated", func() {
					datastore.Put(c, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Zone: "zone",
						},
					})
					err := ensure(c, &tasks.Ensure{
						Id: "id",
						Attributes: &config.VM{
							Project: "project",
						},
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					err = datastore.Get(c, v)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Project: "project",
						},
					})
				})
			})
		})

		Convey("expand", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := expand(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					err := expand(c, &tasks.Expand{})
					So(err, ShouldErrLike, "ID is required")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("missing", func() {
					err := expand(c, &tasks.Expand{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to get VMs block")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				Convey("zero", func() {
					srv.EnsureVMs(c, &config.EnsureVMsRequest{
						Id:  "id",
						Vms: &config.Block{},
					})
					err := expand(c, &tasks.Expand{Id: "id"})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("one", func() {
					srv.EnsureVMs(c, &config.EnsureVMsRequest{
						Id: "id",
						Vms: &config.Block{
							Amount: 1,
						},
					})
					err := expand(c, &tasks.Expand{Id: "id"})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
				})
			})
		})
	})
}
