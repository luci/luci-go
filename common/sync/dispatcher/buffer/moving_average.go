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
	values []int
	idx    int
}

// newMovingAverage makes a new movingAverage
//
// Args:
//   window - The number of values to track.; if this is <= 0, panics.
//   initialGuess - A seed value; if this is <= 0, panics.
func newMovingAverage(window, initialGuess int) *movingAverage {
	if window <= 0 {
		panic(errors.New("window must be positive"))
	}
	if initialGuess <= 0 {
		panic(errors.New("initialGuess must be positive"))
	}

	values := make([]int, window)
	for i := range values {
		values[i] = initialGuess
	}

	return &movingAverage{values, 0}
}

// get retrieves the current average value.
func (m *movingAverage) get() int {
	acc := 0
	for _, value := range m.values {
		acc += value
	}

	// Do this to round up to the next integer value.
	numValues := len(m.values)
	ret := (acc + numValues - 1) / numValues

	// Never return a 0 or negative value.
	if ret <= 0 {
		ret = 1
	}

	return ret
}

// adds a new value to track.
func (m *movingAverage) record(value int) {
	m.values[m.idx] = value
	m.idx = (m.idx + 1) % len(m.values)
}
