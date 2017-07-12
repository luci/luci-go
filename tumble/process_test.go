// Copyright 2015 The LUCI Authors.
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

package tumble

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/service/datastore/serialize"
	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTumbleFiddlyBits(t *testing.T) {
	t.Parallel()

	Convey("Fiddly bits", t, func() {
		tt := &Testing{}
		ctx := tt.Context()
		l := logging.Get(ctx).(*memlogger.MemLogger)

		Convey("early exit logic works", func() {
			future := clock.Now(ctx).UTC().Add(time.Hour)
			itm := mc.NewItem(ctx, fmt.Sprintf("%s.%d.last", baseName, 10)).SetValue(serialize.ToBytes(future))
			So(mc.Set(ctx, itm), ShouldBeNil)

			So(fireTasks(ctx, tt.GetConfig(ctx), map[taskShard]struct{}{
				{10, minTS}: {},
			}, true), ShouldBeTrue)
			tt.Drain(ctx)

			So(l.Has(logging.Info,
				"Processing tumble shard.", map[string]interface{}{"shard": uint64(10)}),
				ShouldBeTrue)
			So(l.Has(logging.Info,
				"early exit, 0001-02-03 05:05:10 +0000 UTC > 0001-02-03 04:05:12 +0000 UTC", nil),
				ShouldBeTrue)
		})
	})
}
