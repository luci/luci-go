// Copyright 2014 The LUCI Authors.
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
	"context"
	"time"
)

// Implementation of Clock that uses Go's standard library.
type systemClock struct{}

// System clock instance.
var systemClockInstance systemClock

var _ Clock = systemClock{}

// GetSystemClock returns an instance of a Clock whose method calls directly use
// Go's "time" library.
func GetSystemClock() Clock {
	return systemClockInstance
}

func (systemClock) Now() time.Time {
	return time.Now()
}

func (sc systemClock) Sleep(ctx context.Context, d time.Duration) TimerResult {
	return <-after(ctx, sc, d)
}

func (systemClock) NewTimer(ctx context.Context) Timer {
	return newSystemTimer(ctx)
}
