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

	. "github.com/smartystreets/goconvey/convey"
)

func TestQuota(t *testing.T) {
	t.Parallel()

	Convey("UpdateQuota", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

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
}
