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

package buildbot

import (
	"encoding/json"
	"time"
)

type Time struct {
	// if we do a type alias, non-pointer time.Time methods are not available

	time.Time
}

// MarshalJSON marshals t to JSON at microsecond resolution.
func (t *Time) MarshalJSON() ([]byte, error) {
	var buildbotFormat *float64
	if !t.Time.IsZero() {
		usec := t.Time.Nanosecond() / 1e3
		// avoid dividing big floating numbers
		v := float64(t.Time.Unix()) + float64(usec)/1e6
		buildbotFormat = &v
	}
	return json.Marshal(&buildbotFormat)
}

// UnmarshalJSON unmarshals t from JSON at microsecond resolution.
func (t *Time) UnmarshalJSON(data []byte) error {
	var buildbotFormat *float64
	err := json.Unmarshal(data, &buildbotFormat)
	if err != nil {
		return err
	}
	*t = Time{}
	if buildbotFormat != nil {
		sec := *buildbotFormat
		usec := int64(sec*1e6) % 1e6
		t.Time = time.Unix(int64(sec), usec*1e3).UTC()
	}
	return nil
}

type TimeRange struct {
	Start, Finish Time
}

// MkTimeRange is a shorthand for:
//   buildbot.TimeRange{Start: start, Finish: finish}
func MkTimeRange(start, finish Time) TimeRange {
	return TimeRange{start, finish}
}

func (t *TimeRange) MarshalJSON() ([]byte, error) {
	buildbotFormat := []Time{t.Start, t.Finish}
	return json.Marshal(buildbotFormat)
}

func (t *TimeRange) UnmarshalJSON(data []byte) error {
	var buildbotFormat []Time
	err := json.Unmarshal(data, &buildbotFormat)
	if err != nil {
		return err
	}

	*t = TimeRange{}
	switch {
	case len(buildbotFormat) > 1:
		t.Finish = buildbotFormat[1]
		fallthrough
	case len(buildbotFormat) > 0:
		t.Start = buildbotFormat[0]
	}
	return nil
}
