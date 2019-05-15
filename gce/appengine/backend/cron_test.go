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
	"fmt"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCron(t *testing.T) {
	t.Parallel()

	Convey("cron", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		c := withDispatcher(memory.Use(context.Background()), dsp)
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("countTasks", func() {
			dsp := &tq.Dispatcher{}
			c = withDispatcher(c, dsp)
			q := datastore.NewQuery("TaskCount")

			Convey("none", func() {
				So(countTasks(c), ShouldBeNil)
				var k []*datastore.Key
				So(datastore.GetAll(c, q, &k), ShouldBeNil)
				So(k, ShouldBeEmpty)
			})

			Convey("many", func() {
				dsp.RegisterTask(&tasks.CountVMs{}, countVMs, countVMsQueue, nil)
				dsp.RegisterTask(&tasks.ManageBot{}, manageBot, manageBotQueue, nil)
				So(countTasks(c), ShouldBeNil)
				var k []*datastore.Key
				So(datastore.GetAll(c, q, &k), ShouldBeNil)
				So(k, ShouldHaveLength, 2)
			})
		})

		Convey("countVMsAsync", func() {
			Convey("none", func() {
				So(countVMsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				So(countVMsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("createInstancesAsync", func() {
			Convey("none", func() {
				Convey("zero", func() {
					So(createInstancesAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("exists", func() {
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					So(createInstancesAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID: "id",
					Attributes: config.VM{
						Zone: "zone",
					},
				})
				So(createInstancesAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("expandConfigsAsync", func() {
			Convey("none", func() {
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Config{
					ID: "id",
					Config: config.Config{
						Amount: &config.Amount{
							Default: 1,
						},
					},
				})
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})

			Convey("many", func() {
				for i := 0; i < 100; i++ {
					datastore.Put(c, &model.Config{
						ID: fmt.Sprintf("id-%d", i),
						Config: config.Config{
							Amount: &config.Amount{
								Default: 1,
							},
						},
					})
				}
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 100)
			})
		})

		Convey("manageBotsAsync", func() {
			Convey("none", func() {
				Convey("missing", func() {
					So(manageBotsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("url", func() {
					datastore.Put(c, &model.VM{
						ID: "id",
					})
					So(manageBotsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				So(manageBotsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("payloadFactory", func() {
			f := payloadFactory(&tasks.CountVMs{})
			p := f("id")
			So(p, ShouldResemble, &tasks.CountVMs{
				Id: "id",
			})
		})

		Convey("reportQuotasAsync", func() {
			Convey("none", func() {
				So(reportQuotasAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Project{
					ID: "id",
				})
				So(reportQuotasAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("trigger", func() {
			Convey("none", func() {
				So(trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID: "id",
				})
				So(trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
				So(tqt.GetScheduledTasks()[0].Payload, ShouldResemble, &tasks.ManageBot{
					Id: "id",
				})
			})
		})
	})
}
