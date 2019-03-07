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

package backend

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	Convey("reportConnected", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))

		instances["name"] = &inst{
			connected: true,
		}
		reportConnected(c)
		fields := []interface{}{"autogen:name", "", "", "", ""}
		So(tsmon.Store(c).Get(c, connected, time.Time{}, fields).(bool), ShouldEqual, true)
		So(instances, ShouldBeEmpty)
	})

	Convey("setConnected", t, func() {
		c := context.Background()

		setConnected(c, true, &model.VM{
			Hostname: "name",
		})
		inst, ok := instances["name"]
		So(ok, ShouldBeTrue)
		So(inst.connected, ShouldBeTrue)

		setConnected(c, false, &model.VM{
			Hostname: "name",
		})
		inst, ok = instances["name"]
		So(ok, ShouldBeTrue)
		So(inst.connected, ShouldBeFalse)
	})
}
