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
	"context"
	"time"
)

// Unique value for clock key.
var clockKey = "clock.Clock"

// Factory is a generator function that produces a Clock instnace.
type Factory func(context.Context) Clock

// SetFactory creates a new Context using the supplied Clock factory.
func SetFactory(ctx context.Context, f Factory) context.Context {
	return context.WithValue(ctx, &clockKey, f)
}

// Set creates a new Context using the supplied Clock.
func Set(ctx context.Context, c Clock) context.Context {
	return SetFactory(ctx, func(context.Context) Clock { return c })
}

// Get returns the Clock set in the supplied Context, defaulting to
// SystemClock() if none is set.
func Get(ctx context.Context) (clock Clock) {
	if v := ctx.Value(&clockKey); v != nil {
		if f, ok := v.(Factory); ok {
			clock = f(ctx)
		}
	}
	if clock == nil {
		clock = GetSystemClock()
	}
	return
}

// Now calls Clock.Now on the Clock instance stored in the supplied Context.
func Now(ctx context.Context) time.Time {
	return Get(ctx).Now()
}

// Sleep calls Clock.Sleep on the Clock instance stored in the supplied Context.
func Sleep(ctx context.Context, d time.Duration) TimerResult {
	return Get(ctx).Sleep(ctx, d)
}

// NewTimer calls Clock.NewTimer on the Clock instance stored in the supplied
// Context.
func NewTimer(ctx context.Context) Timer {
	return Get(ctx).NewTimer(ctx)
}

// After waits a duration using the Clock instance stored in the supplied
// Context. Then sends the current time over the returned channel.
//
// If the supplied Context is canceled, the timer will expire immediately.
func After(ctx context.Context, d time.Duration) <-chan TimerResult {
	return after(ctx, Get(ctx), d)
}

func after(ctx context.Context, c Clock, d time.Duration) <-chan TimerResult {
	t := c.NewTimer(ctx)
	t.Reset(d)
	return t.GetC()
}

// Since is an equivalent of time.Since.
func Since(ctx context.Context, t time.Time) time.Duration {
	return Now(ctx).Sub(t)
}

// Until is an equivalent of time.Until.
func Until(ctx context.Context, t time.Time) time.Duration {
	return t.Sub(Now(ctx))
}
