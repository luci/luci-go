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
	"time"
)

// ExponentialBackoff is an Iterator implementation that implements exponential
// backoff retry.
type ExponentialBackoff struct {
	Limited

	// Multiplier is the exponential growth multiplier. If < 1, a default of 2
	// will be used.
	Multiplier float64
	// MaxDelay is the maximum duration. If <= zero, no maximum will be enforced.
	MaxDelay time.Duration
}

// Next implements Iterator.
func (b *ExponentialBackoff) Next(ctx context.Context, err error) time.Duration {
	// Get Limited base delay.
	delay := b.Limited.Next(ctx, err)
	if delay == Stop {
		return Stop
	}

	// Bound our delay.
	if b.MaxDelay > 0 && b.MaxDelay < delay {
		delay = b.MaxDelay
	} else {
		// Calculate the next delay exponentially.
		multiplier := b.Multiplier
		if multiplier < 1 {
			multiplier = 2
		}
		b.Delay = time.Duration(float64(b.Delay) * multiplier)
	}
	return delay
}
