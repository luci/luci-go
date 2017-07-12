// Copyright 2016 The LUCI Authors.
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

package grpcmon

import (
	"testing"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnaryServerInterceptor(t *testing.T) {
	Convey("Captures count and duration", t, func() {
		c, memStore := testContext()

		// Handler that runs for 500 ms.
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			clock.Get(ctx).(testclock.TestClock).Add(500 * time.Millisecond)
			return struct{}{}, nil
		}

		// Run the handler with the interceptor.
		NewUnaryServerInterceptor(nil)(c, nil, &grpc.UnaryServerInfo{
			FullMethod: "/service/method",
		}, handler)

		count, err := memStore.Get(c, grpcServerCount, time.Time{}, []interface{}{"/service/method", 0})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 1)

		duration, err := memStore.Get(c, grpcServerDuration, time.Time{}, []interface{}{"/service/method", 0})
		So(err, ShouldBeNil)
		So(duration.(*distribution.Distribution).Sum(), ShouldEqual, 500)
	})
}

func testContext() (context.Context, store.Store) {
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	c = gologger.StdConfig.Use(c)
	c, _, _ = tsmon.WithFakes(c)
	memStore := store.NewInMemory(&target.Task{})
	tsmon.GetState(c).SetStore(memStore)
	return c, memStore
}
