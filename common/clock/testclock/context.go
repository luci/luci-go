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

package testclock

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
)

// TestTimeUTC is an arbitrary time point in UTC for testing.
//
// It corresponds to a negative Unix timestamp, so it might be inappropriate for
// some tests. Use 'TestRecentTimeUTC' for more "real" test time in this case.
var TestTimeUTC = time.Date(1, time.February, 3, 4, 5, 6, 7, time.UTC)

// TestRecentTimeUTC is like TestTimeUTC, but in the recent past.
var TestRecentTimeUTC = time.Date(2016, time.February, 3, 4, 5, 6, 7, time.UTC)

// TestTimeLocal is an arbitrary time point in the 'Local' time zone for
// testing.
//
// It corresponds to a negative Unix timestamp, so it might be inappropriate for
// some tests. Use 'TestRecentTimeLocal' for more "real" test time in this case.
var TestTimeLocal = time.Date(1, time.February, 3, 4, 5, 6, 7, time.Local)

// TestRecentTimeLocal is like TestTimeLocal, but in the recent past.
var TestRecentTimeLocal = time.Date(2016, time.February, 3, 4, 5, 6, 7, time.Local)

// UseTime instantiates a TestClock and returns a Context that is configured to
// use that clock, as well as the instantiated clock.
func UseTime(ctx context.Context, now time.Time) (context.Context, TestClock) {
	tc := New(now)
	return clock.Set(ctx, tc), tc
}
