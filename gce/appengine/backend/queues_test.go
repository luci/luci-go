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
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueues(t *testing.T) {
	t.Parallel()

	Convey("queues", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		c := withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv)
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("drain", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := drain(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
				})

				Convey("empty", func() {
					err := drain(c, &tasks.Drain{})
					So(err, ShouldErrLike, "ID is required")
				})
			})

			Convey("valid", func() {
				Convey("drained", func() {
					datastore.Put(c, &model.VM{
						ID:      "id-2",
						Drained: false,
					})
					err := drain(c, &tasks.Drain{
						Id: "id-2",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id-2",
					}
					datastore.Get(c, v)
					So(v.Drained, ShouldBeTrue)
				})

				Convey("non-existent", func() {
					err := drain(c, &tasks.Drain{
						Id: "id-2",
					})
					So(err, ShouldBeNil)
					err = datastore.Get(c, &model.VM{
						ID: "id-2",
					})
					So(err, ShouldEqual, datastore.ErrNoSuchEntity)
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
					So(err, ShouldErrLike, "VMs is required")
				})
			})

			Convey("valid", func() {
				Convey("nil", func() {
					err := ensure(c, &tasks.Ensure{
						Index: 2,
						Vms:   "id",
					})
					So(err, ShouldBeNil)
					err = datastore.Get(c, &model.VM{
						ID: "id-2",
					})
					So(err, ShouldBeNil)
				})

				Convey("empty", func() {
					err := ensure(c, &tasks.Ensure{
						Attributes: &config.VM{},
						Index:      2,
						Vms:        "id",
					})
					So(err, ShouldBeNil)
					err = datastore.Get(c, &model.VM{
						ID:      "id-2",
						Drained: false,
					})
					So(err, ShouldBeNil)
				})

				Convey("non-empty", func() {
					err := ensure(c, &tasks.Ensure{
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
						Index: 2,
						Vms:   "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id-2",
					}
					err = datastore.Get(c, v)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id-2",
						Attributes: config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
						Drained: false,
						Index:   2,
						VMs:     "id",
					})
				})

				Convey("updated", func() {
					datastore.Put(c, &model.VM{
						ID: "id-0",
						Attributes: config.VM{
							Zone: "zone",
						},
						Drained: true,
					})
					err := ensure(c, &tasks.Ensure{
						Attributes: &config.VM{
							Project: "project",
						},
						Index: 2,
						Vms:   "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id-2",
					}
					err = datastore.Get(c, v)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id-2",
						Attributes: config.VM{
							Project: "project",
						},
						Drained: false,
						Index:   2,
						VMs:     "id",
					})
				})
			})
		})

		Convey("process", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := process(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					err := process(c, &tasks.Process{})
					So(err, ShouldErrLike, "ID is required")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("missing", func() {
					err := process(c, &tasks.Process{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to get VMs block")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				Convey("none", func() {
					srv.EnsureVMs(c, &config.EnsureVMsRequest{
						Id:  "id",
						Vms: &config.Block{},
					})
					err := process(c, &tasks.Process{Id: "id"})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("ensure", func() {
					srv.EnsureVMs(c, &config.EnsureVMsRequest{
						Id: "id",
						Vms: &config.Block{
							Amount: 3,
						},
					})
					err := process(c, &tasks.Process{Id: "id"})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 3)
					task, ok := tqt.GetScheduledTasks()[2].Payload.(*tasks.Ensure)
					So(ok, ShouldBeTrue)
					So(task.Index, ShouldEqual, 2)
					So(task.Vms, ShouldEqual, "id")
				})

				Convey("drain", func() {
					datastore.Put(c, &model.VM{
						ID:    "id-2",
						Index: 2,
						VMs:   "id",
					})
					srv.EnsureVMs(c, &config.EnsureVMsRequest{
						Id: "id",
						Vms: &config.Block{
							Amount: 0,
						},
					})
					err := process(c, &tasks.Process{Id: "id"})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					task, ok := tqt.GetScheduledTasks()[0].Payload.(*tasks.Drain)
					So(ok, ShouldBeTrue)
					So(task.Id, ShouldEqual, "id-2")
				})
			})
		})
	})
}
