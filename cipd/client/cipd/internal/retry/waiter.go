// Copyright 2019 The LUCI Authors.
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

// Package retry contains helpers for doing tight retry loops.
package retry

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Waiter returns a stateful callback which sleeps a bit on each invocation
// until reaching a deadline.
//
// It also returns a cancel function which must be called when abandoning
// the waiter to disarm any pending timers.
func Waiter(ctx context.Context, why string, d time.Duration) (sleep func() error, cancel func()) {
	var attempt int32
	ctx, cancel = clock.WithTimeout(ctx, d)
	return func() error {
		if attempt++; attempt > 50 {
			attempt = 50 // cap sleeping time at max 5 sec
		}
		delay := time.Duration(rand.Int31n(100*attempt)) * time.Millisecond
		if attempt > 10 {
			logging.Warningf(ctx, "%s: retrying after %s...", why, delay)
		} else {
			logging.Debugf(ctx, "%s: retrying after %s...", why, delay)
		}
		tr := clock.Sleep(ctx, delay)
		return tr.Err
	}, cancel
}
