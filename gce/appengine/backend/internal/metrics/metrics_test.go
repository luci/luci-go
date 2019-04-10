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

package metrics

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	Convey("Metrics", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		Convey("InstanceCounter", func() {
			ic := make(InstanceCounter)
			So(ic, ShouldBeEmpty)

			ic.Configured(3, "project")
			ic.Created(1, "project", "zone-1")
			ic.Created(1, "project", "zone-1")
			ic.Created(1, "project", "zone-2")
			ic.Connected(1, "project", "server", "zone-1")
			So(ic.Update(c, "prefix"), ShouldBeNil)

			n := &InstanceCount{
				ID: "prefix",
			}
			So(datastore.Get(c, n), ShouldBeNil)
			So(n.Counts, ShouldHaveLength, 4)
			So(n.Counts, ShouldContain, instanceCount{
				Configured: 3,
				Project:    "project",
			})
			So(n.Counts, ShouldContain, instanceCount{
				Created: 2,
				Project: "project",
				Zone:    "zone-1",
			})
			So(n.Counts, ShouldContain, instanceCount{
				Created: 1,
				Project: "project",
				Zone:    "zone-2",
			})
			So(n.Counts, ShouldContain, instanceCount{
				Connected: 1,
				Project:   "project",
				Server:    "server",
				Zone:      "zone-1",
			})
		})

		Convey("UpdateFailures", func() {
			fields := []interface{}{"prefix", "project", "zone"}

			UpdateFailures(c, 1, &model.VM{
				Attributes: config.VM{
					Project: "project",
					Zone:    "zone",
				},
				Hostname: "name-1",
				Prefix:   "prefix",
			})
			So(s.Get(c, creationFailures, time.Time{}, fields).(int64), ShouldEqual, 1)

			UpdateFailures(c, 1, &model.VM{
				Attributes: config.VM{
					Project: "project",
					Zone:    "zone",
				},
				Hostname: "name-1",
				Prefix:   "prefix",
			})
			So(s.Get(c, creationFailures, time.Time{}, fields).(int64), ShouldEqual, 2)
		})

		Convey("updateInstances", func() {
			fields := []interface{}{"prefix", "project"}

			Convey("expired", func() {
				datastore.Put(c, &InstanceCount{
					ID:       "prefix",
					Computed: time.Time{},
					Counts: []instanceCount{
						{
							Configured: 1,
							Project:    "project",
						},
					},
					Prefix: "prefix",
				})
				updateInstances(c)
				So(s.Get(c, configuredInstances, time.Time{}, fields), ShouldBeNil)
				n := &InstanceCount{
					ID: "prefix",
				}
				So(datastore.Get(c, n), ShouldEqual, datastore.ErrNoSuchEntity)
			})

			Convey("valid", func() {
				datastore.Put(c, &InstanceCount{
					ID:       "prefix",
					Computed: time.Now().UTC(),
					Counts: []instanceCount{
						{
							Configured: 1,
							Project:    "project",
						},
					},
					Prefix: "prefix",
				})
				updateInstances(c)
				So(s.Get(c, configuredInstances, time.Time{}, fields).(int64), ShouldEqual, 1)
				n := &InstanceCount{
					ID: "prefix",
				}
				So(datastore.Get(c, n), ShouldBeNil)

				datastore.Put(c, &InstanceCount{
					ID:       "prefix",
					Computed: time.Now().UTC(),
					Counts: []instanceCount{
						{
							Configured: 0,
							Project:    "project",
						},
					},
					Prefix: "prefix",
				})
				updateInstances(c)
				So(s.Get(c, configuredInstances, time.Time{}, fields).(int64), ShouldEqual, 0)
				n = &InstanceCount{
					ID: "prefix",
				}
				So(datastore.Get(c, n), ShouldBeNil)
			})
		})

		Convey("UpdateQuota", func() {
			fields := []interface{}{"metric", "region", "project"}

			UpdateQuota(c, 100.0, 25.0, "metric", "region", "project")
			So(s.Get(c, quotaLimit, time.Time{}, fields).(float64), ShouldEqual, 100.0)
			So(s.Get(c, quotaRemaining, time.Time{}, fields).(float64), ShouldEqual, 75.0)
			So(s.Get(c, quotaUsage, time.Time{}, fields).(float64), ShouldEqual, 25.0)

			UpdateQuota(c, 120.0, 40.0, "metric", "region", "project")
			So(s.Get(c, quotaLimit, time.Time{}, fields).(float64), ShouldEqual, 120.0)
			So(s.Get(c, quotaRemaining, time.Time{}, fields).(float64), ShouldEqual, 80.0)
			So(s.Get(c, quotaUsage, time.Time{}, fields).(float64), ShouldEqual, 40.0)
		})
	})
}
