// Copyright 2017 The LUCI Authors.
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

package gs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRetryAndStatusCode(t *testing.T) {
	t.Parallel()

	Convey("With clock", t, func() {
		ctx, cl := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		cl.SetTimerCallback(func(d time.Duration, t clock.Timer) { cl.Add(d) })

		Convey("Happy path", func() {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return nil
			})
			So(calls, ShouldEqual, 1)
			So(err, ShouldBeNil)
			So(StatusCode(err), ShouldEqual, 200)
		})

		Convey("Retying generic connection error", func() {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return fmt.Errorf("generic error")
			})
			So(calls, ShouldEqual, 11)
			So(err, ShouldErrLike, "generic error")
			So(transient.Tag.In(err), ShouldBeTrue)
			So(StatusCode(err), ShouldEqual, 0)
		})

		Convey("Retying transient API error", func() {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return &googleapi.Error{Code: 500}
			})
			So(calls, ShouldEqual, 11)
			So(err, ShouldErrLike, "HTTP code 500")
			So(transient.Tag.In(err), ShouldBeTrue)
			So(StatusCode(err), ShouldEqual, 500)
		})

		Convey("Giving up on fatal API error", func() {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return &googleapi.Error{Code: 403}
			})
			So(calls, ShouldEqual, 1)
			So(err, ShouldErrLike, "HTTP code 403")
			So(transient.Tag.In(err), ShouldBeFalse)
			So(StatusCode(err), ShouldEqual, 403)
		})

		Convey("Passes through *RestartUploadError", func() {
			uploadErr := &RestartUploadError{Offset: 123}
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return uploadErr
			})
			So(calls, ShouldEqual, 1)
			So(err, ShouldEqual, uploadErr) // exact same error object
		})
	})
}
