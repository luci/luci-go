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

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstances(t *testing.T) {
	t.Parallel()

	Convey("Instances", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		Convey("InstanceCount", func() {
			ic := &InstanceCount{
				ScalingType: "dynamic",
			}
			So(ic.Configured, ShouldBeEmpty)
			So(ic.Created, ShouldBeEmpty)
			So(ic.Connected, ShouldBeEmpty)

			Convey("Configured", func() {
				ic.AddConfigured(1, "project-1")
				ic.AddConfigured(1, "project-2")
				ic.AddConfigured(1, "project-1")
				So(ic.Configured, ShouldHaveLength, 2)
				So(ic.Configured, ShouldContain, configuredCount{
					Count:   2,
					Project: "project-1",
				})
				So(ic.Configured, ShouldContain, configuredCount{
					Count:   1,
					Project: "project-2",
				})
			})

			Convey("Created", func() {
				ic.AddCreated(1, "project", "zone-1")
				ic.AddCreated(1, "project", "zone-2")
				ic.AddCreated(1, "project", "zone-1")
				So(ic.Created, ShouldHaveLength, 2)
				So(ic.Created, ShouldContain, createdCount{
					Count:   2,
					Project: "project",
					Zone:    "zone-1",
				})
				So(ic.Created, ShouldContain, createdCount{
					Count:   1,
					Project: "project",
					Zone:    "zone-2",
				})
			})

			Convey("Connected", func() {
				ic.AddConnected(1, "project", "server-1", "zone")
				ic.AddConnected(1, "project", "server-2", "zone")
				ic.AddConnected(1, "project", "server-1", "zone")
				So(ic.Connected, ShouldHaveLength, 2)
				So(ic.Connected, ShouldContain, connectedCount{
					Count:   2,
					Project: "project",
					Server:  "server-1",
					Zone:    "zone",
				})
				So(ic.Connected, ShouldContain, connectedCount{
					Count:   1,
					Project: "project",
					Server:  "server-2",
					Zone:    "zone",
				})
			})

			Convey("Update", func() {
				ic.AddConfigured(1, "project")
				ic.AddCreated(1, "project", "zone")
				ic.AddConnected(1, "project", "server", "zone")
				So(ic.Update(c, "prefix", "resource_group", "dynamic"), ShouldBeNil)
				ic = &InstanceCount{
					ID: "prefix",
				}
				So(datastore.Get(c, ic), ShouldBeNil)
				So(ic.Configured, ShouldHaveLength, 1)
				So(ic.Created, ShouldHaveLength, 1)
				So(ic.Connected, ShouldHaveLength, 1)
				So(ic.Prefix, ShouldEqual, ic.ID)
				So(ic.ResourceGroup, ShouldEqual, "resource_group")
				So(ic.ScalingType, ShouldEqual, "dynamic")
			})
		})

		Convey("updateInstances", func() {
			confFields := []any{"prefix", "project", "resource_group", "dynamic"}
			creaFields1 := []any{"prefix", "project", "dynamic", "zone-1"}
			creaFields2 := []any{"prefix", "project", "dynamic", "zone-2"}
			connFields := []any{"prefix", "project", "dynamic", "resource_group", "server", "zone"}

			ic := &InstanceCount{
				ID:            "prefix",
				ResourceGroup: "resource_group",
				ScalingType:   "dynamic",
				Configured: []configuredCount{
					{
						Count:   3,
						Project: "project",
					},
				},
				Created: []createdCount{
					{
						Count:   2,
						Project: "project",
						Zone:    "zone-1",
					},
					{
						Count:   1,
						Project: "project",
						Zone:    "zone-2",
					},
				},
				Connected: []connectedCount{
					{
						Count:   1,
						Project: "project",
						Server:  "server",
						Zone:    "zone",
					},
				},
				Prefix: "prefix",
			}

			ic.Computed = time.Time{}
			So(datastore.Put(c, ic), ShouldBeNil)
			updateInstances(c)
			So(s.Get(c, configuredInstances, time.Time{}, confFields), ShouldBeNil)
			So(s.Get(c, createdInstances, time.Time{}, creaFields1), ShouldBeNil)
			So(s.Get(c, createdInstances, time.Time{}, creaFields2), ShouldBeNil)
			So(s.Get(c, connectedInstances, time.Time{}, connFields), ShouldBeNil)
			So(datastore.Get(c, &InstanceCount{
				ID: ic.ID,
			}), ShouldEqual, datastore.ErrNoSuchEntity)

			ic.Computed = time.Now().UTC()
			So(datastore.Put(c, ic), ShouldBeNil)
			updateInstances(c)
			So(s.Get(c, configuredInstances, time.Time{}, confFields).(int64), ShouldEqual, 3)
			So(s.Get(c, createdInstances, time.Time{}, creaFields1).(int64), ShouldEqual, 2)
			So(s.Get(c, createdInstances, time.Time{}, creaFields2).(int64), ShouldEqual, 1)
			So(s.Get(c, connectedInstances, time.Time{}, connFields).(int64), ShouldEqual, 1)
			So(datastore.Get(c, &InstanceCount{
				ID: ic.ID,
			}), ShouldBeNil)
		})
	})
}
