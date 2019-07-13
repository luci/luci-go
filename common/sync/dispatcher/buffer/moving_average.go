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

package buffer

import (
	"errors"
)

// movingAverage tracks a moving average of integers.
//
// It is not goroutine-safe.
type movingAverage struct {
	values []int // used as a circular buffer
	curIdx int
	curSum int64
}

// newMovingAverage makes a new movingAverage
//
// Args:
//   window - The number of values to track.; if this is <= 0, panics.
//   seed - A seed value.
func newMovingAverage(window, seed int) *movingAverage {
	if window <= 0 {
		panic(errors.New("window must be positive"))
	}

	values := make([]int, window)
	curSum := int64(0)
	for i := range values {
		values[i] = seed
		curSum += int64(seed)
	}

	return &movingAverage{values, 0, curSum}
}

// get retrieves the current average value.
func (m *movingAverage) get() float64 {
	return float64(m.curSum) / float64(len(m.values))
}

// adds a new value to track.
func (m *movingAverage) record(value int) {
	m.curSum += int64(value) - int64(m.values[m.curIdx])
	m.values[m.curIdx] = value
	m.curIdx = (m.curIdx + 1) % len(m.values)
}
