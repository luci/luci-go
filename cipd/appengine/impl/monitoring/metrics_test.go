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

package monitoring

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	api "go.chromium.org/luci/cipd/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	ctx, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
	ctx = caching.WithEmptyProcessCache(ctx)

	s := tsmon.Store(ctx)
	fields := []any{"bots", "anonymous:anonymous", "GCS"}

	Convey("FileSize", t, func() {
		So(cachedCfg.Set(ctx, &api.ClientMonitoringWhitelist{
			ClientMonitoringConfig: []*api.ClientMonitoringConfig{
				{IpWhitelist: "bots", Label: "bots"},
			},
		}, nil), ShouldBeNil)

		Convey("not configured", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{})
			FileSize(ctx, 123)
			So(s.Get(ctx, bytesRequested, time.Time{}, fields), ShouldBeNil)
		})

		Convey("configured", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				PeerIPAllowlist: []string{"bots"},
			})
			FileSize(ctx, 123)
			So(s.Get(ctx, bytesRequested, time.Time{}, fields).(int64), ShouldEqual, 123)
			FileSize(ctx, 1)
			So(s.Get(ctx, bytesRequested, time.Time{}, fields).(int64), ShouldEqual, 124)
		})
	})
}
