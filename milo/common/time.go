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

package common

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type Interval struct {
	Start time.Time
	End   time.Time
	Now   time.Time
}

func (in Interval) Started() bool {
	return !in.Start.IsZero()
}

func (in Interval) Ended() bool {
	return !in.End.IsZero()
}

func (in Interval) Duration() time.Duration {
	// An interval that hasn't started has no duration.
	if !in.Started() {
		return 0
	}

	end := in.End
	if end.IsZero() {
		// Open Interval
		end = in.Now
	}

	// Don't return non-sensical values.
	if d := end.Sub(in.Start); d > 0 {
		return d
	}
	return 0
}

func ToInterval(start, end, now *timestamp.Timestamp) (result Interval) {
	if t, err := ptypes.Timestamp(start); err == nil {
		result.Start = t
	}
	if t, err := ptypes.Timestamp(end); err == nil {
		result.End = t
	}
	if t, err := ptypes.Timestamp(now); err == nil {
		result.Now = t
	}
	return
}

// Duration returns duration between start and the earliest of ends.
// Ignores nil ends.
// Fallbacks to "N/A" on insufficient data.
func Duration(start *timestamp.Timestamp, ends ...*timestamp.Timestamp) string {
	if start != nil {
		startTime, _ := ptypes.Timestamp(start)

		var earliestEnd time.Time
		for _, ts := range ends {
			if ts == nil {
				continue
			}
			t, _ := ptypes.Timestamp(ts)
			if earliestEnd.IsZero() || t.Before(earliestEnd) {
				earliestEnd = t
			}
		}

		if startTime.Before(earliestEnd) {
			return HumanDuration(earliestEnd.Sub(startTime))
		}
	}

	return "N/A"
}

// HumanDuration translates d into a human readable string of x units y units,
// where x and y could be in days, hours, minutes, or seconds, whichever is the
// largest.
func HumanDuration(d time.Duration) string {
	t := int64(d.Seconds())
	day := t / 86400
	hr := (t % 86400) / 3600

	if day > 0 {
		if hr != 0 {
			return fmt.Sprintf("%d days %d hrs", day, hr)
		}
		return fmt.Sprintf("%d days", day)
	}

	min := (t % 3600) / 60
	if hr > 0 {
		if min != 0 {
			return fmt.Sprintf("%d hrs %d mins", hr, min)
		}
		return fmt.Sprintf("%d hrs", hr)
	}

	sec := t % 60
	if min > 0 {
		if sec != 0 {
			return fmt.Sprintf("%d mins %d secs", min, sec)
		}
		return fmt.Sprintf("%d mins", min)
	}

	if sec != 0 {
		return fmt.Sprintf("%d secs", sec)
	}

	if d > time.Millisecond {
		return fmt.Sprintf("%d ms", d/time.Millisecond)
	}

	return "0"
}
