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

package clock

import (
	"time"
)

// Timer is a wrapper around the time.Timer structure.
//
// A Timer is instantiated from a Clock instance and started via its Reset()
// method.
type Timer interface {
	// GetC returns the underlying timer's channel.
	//
	// If the Timer is interrupted via Stop, its channel will block indefinitely.
	GetC() <-chan TimerResult

	// Reset configures the timer to expire after a specified duration.
	//
	// If the timer is already running, its previous state will be cleared and
	// this method will return true. The channel returned by GetC() will not
	// change due to Reset.
	Reset(d time.Duration) bool

	// Stop clears any timer tasking, rendering it inactive.
	//
	// Stop may be called on an inactive timer, in which case nothing will happen
	// If the timer is active, it will be stopped and this method will return
	// true.
	//
	// If a timer is stopped, its GetC channel will block indefinitely to avoid
	// erroneously unblocking goroutines that are waiting on it. This is
	// consistent with time.Timer.
	Stop() bool
}

// TimerResult is the result for a timer operation.
//
// Time will be set to the time when the result was generated. If the source of
// the result was prematurely terminated due to Context cancellation, Err will
// be non-nil, and will indicate the cancellation reason.
type TimerResult struct {
	time.Time

	// Err, if not nil, indicates that After did not finish naturally and contains
	// the reason why.
	Err error
}

// Incomplete will return true if the timer result indicates that the timer
// operation was canceled prematurely due to Context cancellation or deadline
// expiration.
func (tr TimerResult) Incomplete() bool {
	return tr.Err != nil
}
