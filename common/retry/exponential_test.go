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

package retry

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()

	Convey(`An ExponentialBackoff Iterator, using an instrumented context`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		l := ExponentialBackoff{}

		Convey(`When empty, will Stop immediately.`, func() {
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})

		Convey(`Will delay exponentially.`, func() {
			l.Retries = 4
			l.Delay = time.Second
			So(l.Next(ctx, nil), ShouldEqual, 1*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 2*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 4*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 8*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})

		Convey(`Will bound exponential delay when MaxDelay is set.`, func() {
			l.Retries = 4
			l.Delay = time.Second
			l.MaxDelay = 4 * time.Second
			So(l.Next(ctx, nil), ShouldEqual, 1*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 2*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 4*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, 4*time.Second)
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})
	})
}
