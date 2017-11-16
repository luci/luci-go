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

package lib

import (
	"time"

	"github.com/danjacques/gofslock/fslock"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
)

// createPollingBlocker creates a blocker that polls to see if the lock is available
// at the specified interval and cancels if the specified context is canceled.
func createPollingBlocker(ctx context.Context, pollingInterval time.Duration) fslock.Blocker {
	return func() error {
		if clock.Sleep(ctx, pollingInterval).Err != nil {
			return fslock.ErrLockHeld
		}

		// Returning nil signals that the lock should be retried.
		return nil
	}
}
