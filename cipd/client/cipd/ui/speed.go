// Copyright 2021 The LUCI Authors.
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

package ui

import (
	"time"
)

// speedGauge measures speed using exponential averaging.
type speedGauge struct {
	speed   float64 // the last measured speed or -1 if not yet known
	prevTS  time.Time
	prevVal int64
	samples int
}

func (s *speedGauge) reset(ts time.Time, val int64) {
	s.speed = -1
	s.prevTS = ts
	s.prevVal = val
	s.samples = 0
}

func (s *speedGauge) advance(ts time.Time, val int64) {
	dt := ts.Sub(s.prevTS)
	if dt < 200*time.Millisecond {
		return // too soon
	}

	v := float64(val-s.prevVal) / dt.Seconds()

	s.prevTS = ts
	s.prevVal = val
	s.samples++

	// Apply exponential average. Take the first sample as a base.
	if s.samples == 1 {
		s.speed = v
	} else {
		s.speed = 0.07*v + 0.93*s.speed
	}
}
