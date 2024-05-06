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

func TestLimited(t *testing.T) {
	t.Parallel()

	Convey(`A Limited Iterator, using an instrumented context`, t, func() {
		ctx, clock := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		l := Limited{}

		Convey(`When empty, will return Stop immediately..`, func() {
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})

		Convey(`With 3 retries, will Stop after three retries.`, func() {
			l.Delay = time.Second
			l.Retries = 3

			So(l.Next(ctx, nil), ShouldEqual, time.Second)
			So(l.Next(ctx, nil), ShouldEqual, time.Second)
			So(l.Next(ctx, nil), ShouldEqual, time.Second)
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})

		Convey(`Will stop after MaxTotal.`, func() {
			l.Retries = 1000
			l.Delay = 3 * time.Second
			l.MaxTotal = 8 * time.Second

			So(l.Next(ctx, nil), ShouldEqual, 3*time.Second)
			clock.Add(8 * time.Second)
			So(l.Next(ctx, nil), ShouldEqual, Stop)
		})
	})
}
