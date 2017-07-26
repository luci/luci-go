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
)

func minDuration(d1 time.Duration, d2 time.Duration) time.Duration {
	if d1.Nanoseconds() <= d2.Nanoseconds() {
		return d1
	} else {
		return d2
	}
}

func CreateBlockerUntil(t time.Time, pollingInterval time.Duration) fslock.Blocker {
	return func() error {
		timeRemaining := t.Sub(time.Now())
		if timeRemaining <= 0 {
			return fslock.ErrLockHeld
		}

		time.Sleep(minDuration(pollingInterval, timeRemaining))

		// Returning nil signals that the lock should be retried.
		return nil
	}
}
