// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testclock

import (
	"time"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// TestTimeUTC is an arbitrary time point in UTC for testing.
var TestTimeUTC = time.Date(1, time.February, 3, 4, 5, 6, 7, time.UTC)

// TestTimeLocal is an arbitrary time point in the 'Local' time zone for
// testing.
var TestTimeLocal = time.Date(1, time.February, 3, 4, 5, 6, 7, time.Local)

// UseTime instantiates a TestClock and returns a Context that is configured to
// use that clock, as well as the instantiated clock.
func UseTime(ctx context.Context, now time.Time) (context.Context, TestClock) {
	tc := New(now)
	return clock.Set(ctx, tc), tc
}
