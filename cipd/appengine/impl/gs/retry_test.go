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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRetryAndStatusCode(t *testing.T) {
	t.Parallel()

	ftt.Run("With clock", t, func(t *ftt.Test) {
		ctx, cl := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		cl.SetTimerCallback(func(d time.Duration, t clock.Timer) { cl.Add(d) })

		t.Run("Happy path", func(t *ftt.Test) {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return nil
			})
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, StatusCode(err), should.Equal(200))
		})

		t.Run("Retrying generic connection error", func(t *ftt.Test) {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return fmt.Errorf("generic error")
			})
			assert.Loosely(t, calls, should.Equal(11))
			assert.Loosely(t, err, should.ErrLike("generic error"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, StatusCode(err), should.BeZero)
		})

		t.Run("Retrying transient API error", func(t *ftt.Test) {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return &googleapi.Error{Code: 500}
			})
			assert.Loosely(t, calls, should.Equal(11))
			assert.Loosely(t, err, should.ErrLike("HTTP code 500"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, StatusCode(err), should.Equal(500))
		})

		t.Run("Giving up on fatal API error", func(t *ftt.Test) {
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return &googleapi.Error{Code: 403}
			})
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("HTTP code 403"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			assert.Loosely(t, StatusCode(err), should.Equal(403))
		})

		t.Run("Passes through *RestartUploadError", func(t *ftt.Test) {
			uploadErr := &RestartUploadError{Offset: 123}
			calls := 0
			err := withRetry(ctx, func() error {
				calls++
				return uploadErr
			})
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, err, should.Equal(uploadErr)) // exact same error object
		})
	})
}
