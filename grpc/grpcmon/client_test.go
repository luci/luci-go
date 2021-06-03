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
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon/distribution"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnaryClientInterceptor(t *testing.T) {
	Convey("Captures count and duration", t, func() {
		c, memStore := testContext()
		run := func(err error, dur time.Duration) {
			method := "/service/method"
			invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
				clock.Get(ctx).(testclock.TestClock).Add(dur)
				return err
			}
			_ = NewUnaryClientInterceptor(nil)(c, method, nil, nil, nil, invoker)
		}
		count := func(code string) int64 {
			return memStore.Get(c, grpcClientCount, time.Time{}, []interface{}{"/service/method", code}).(int64)
		}
		duration := func(code string) float64 {
			val := memStore.Get(c, grpcClientDuration, time.Time{}, []interface{}{"/service/method", code})
			return val.(*distribution.Distribution).Sum()
		}

		run(nil, time.Millisecond)
		So(count("OK"), ShouldEqual, 1)
		So(duration("OK"), ShouldEqual, 1)

		run(status.Error(codes.PermissionDenied, "no permission"), time.Second)
		So(count("PERMISSION_DENIED"), ShouldEqual, 1)
		So(duration("PERMISSION_DENIED"), ShouldEqual, 1000)

		run(status.Error(codes.Unauthenticated, "no auth"), time.Minute)
		So(count("UNAUTHENTICATED"), ShouldEqual, 1)
		So(duration("UNAUTHENTICATED"), ShouldEqual, 60000)
	})
}
