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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon/distribution"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnaryClientInterceptor(t *testing.T) {
	Convey("Captures count and duration", t, func() {
		c, memStore := testContext()

		// Fake call that runs for 500 ms.
		invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			clock.Get(ctx).(testclock.TestClock).Add(500 * time.Millisecond)
			return nil
		}

		// Run the call with the interceptor.
		NewUnaryClientInterceptor(nil)(c, "/service/method", nil, nil, nil, invoker)

		count := memStore.Get(c, grpcClientCount, time.Time{}, []interface{}{"/service/method", 0})
		So(count, ShouldEqual, 1)

		duration := memStore.Get(c, grpcClientDuration, time.Time{}, []interface{}{"/service/method", 0})
		So(duration.(*distribution.Distribution).Sum(), ShouldEqual, 500)
	})
}
