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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLimited(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Limited Iterator, using an instrumented context`, t, func(t *ftt.Test) {
		ctx, clock := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		l := Limited{}

		t.Run(`When empty, will return Stop immediately..`, func(t *ftt.Test) {
			assert.Loosely(t, l.Next(ctx, nil), should.Equal(Stop))
		})

		t.Run(`With 3 retries, will Stop after three retries.`, func(t *ftt.Test) {
			l.Delay = time.Second
			l.Retries = 3

			assert.Loosely(t, l.Next(ctx, nil), should.Equal(time.Second))
			assert.Loosely(t, l.Next(ctx, nil), should.Equal(time.Second))
			assert.Loosely(t, l.Next(ctx, nil), should.Equal(time.Second))
			assert.Loosely(t, l.Next(ctx, nil), should.Equal(Stop))
		})

		t.Run(`Will stop after MaxTotal.`, func(t *ftt.Test) {
			l.Retries = 1000
			l.Delay = 3 * time.Second
			l.MaxTotal = 8 * time.Second

			assert.Loosely(t, l.Next(ctx, nil), should.Equal(3*time.Second))
			clock.Add(8 * time.Second)
			assert.Loosely(t, l.Next(ctx, nil), should.Equal(Stop))
		})
	})
}
