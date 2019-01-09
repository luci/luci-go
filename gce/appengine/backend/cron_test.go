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

		Convey("createInstances", func() {
			Convey("none", func() {
				Convey("zero", func() {
					err := createInstances(c)
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("exists", func() {
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := createInstances(c)
					So(err, ShouldBeNil)
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
				err := createInstances(c)
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("processVMs", func() {
			Convey("none", func() {
				err := processVMs(c)
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
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
				err := processVMs(c)
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})

			Convey("many", func() {
				for i := 0; i < 100; i++ {
					datastore.Put(c, &model.VMs{
						ID: fmt.Sprintf("id-%d", i),
						Config: config.Block{
							Amount: 1,
						},
					})
				}
				err := processVMs(c)
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 100)
			})
		})

		Convey("manageInstances", func() {
			Convey("none", func() {
				Convey("zero", func() {
					err := manageInstances(c)
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("not created", func() {
					datastore.Put(c, &model.VM{
						ID: "id",
					})
					err := manageInstances(c)
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manageInstances(c)
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})
	})
}
