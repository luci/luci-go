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
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	Convey("setCached", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))

		cache["name"] = &connInst{
			connected: true,
			prefix:    "prefix",
			project:   "project",
			server:    "server",
			zone:      "zone",
		}
		setCached(c)
		fields := []interface{}{"autogen:name", "prefix", "project", "server", "zone"}
		So(tsmon.Store(c).Get(c, instanceConnected, time.Time{}, fields).(bool), ShouldEqual, true)
		So(cache, ShouldBeEmpty)
	})

	Convey("UpdateQuota", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))

		UpdateQuota(c, 100.0, 25.0, "metric", "region", "project")
		fields := []interface{}{"metric", "region", "project"}
		s := tsmon.Store(c)
		So(s.Get(c, quotaLimit, time.Time{}, fields).(float64), ShouldEqual, 100.0)
		So(s.Get(c, quotaRemaining, time.Time{}, fields).(float64), ShouldEqual, 75.0)
		So(s.Get(c, quotaUsage, time.Time{}, fields).(float64), ShouldEqual, 25.0)
	})

	Convey("UpdateInstance", t, func() {
		c := context.Background()

		UpdateInstance(c, true, &model.VM{
			Attributes: config.VM{
				Project: "project 1",
				Zone:    "zone 1",
			},
			Hostname: "name",
			Prefix:   "prefix 1",
			Swarming: "server 1",
		})
		inst, ok := cache["name"]
		So(ok, ShouldBeTrue)
		So(inst.connected, ShouldBeTrue)
		So(inst.prefix, ShouldEqual, "prefix 1")
		So(inst.project, ShouldEqual, "project 1")
		So(inst.server, ShouldEqual, "server 1")
		So(inst.zone, ShouldEqual, "zone 1")

		UpdateInstance(c, false, &model.VM{
			Attributes: config.VM{
				Project: "project 2",
				Zone:    "zone 2",
			},
			Hostname: "name",
			Prefix:   "prefix 2",
			Swarming: "server 2",
		})
		inst, ok = cache["name"]
		So(ok, ShouldBeTrue)
		So(inst.connected, ShouldBeFalse)
		So(inst.prefix, ShouldEqual, "prefix 2")
		So(inst.project, ShouldEqual, "project 2")
		So(inst.server, ShouldEqual, "server 2")
		So(inst.zone, ShouldEqual, "zone 2")
	})
}
