// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testclock

import (
	"time"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// UseTime instantiates a TestClock and returns a Context that is configured to
// use that clock, as well as the instantiated clock.
func UseTime(ctx context.Context, now time.Time) (context.Context, TestClock) {
	tc := New(now)
	return clock.Set(ctx, tc), tc
}
