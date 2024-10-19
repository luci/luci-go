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

package clockflag

import (
	"encoding/json"
	"flag"
	"time"
)

// Time is a flag- and JSON-compatible Time which parses from RFC3339 strings.
type Time time.Time

var _ interface {
	flag.Value
	json.Unmarshaler
	json.Marshaler
} = (*Time)(nil)

// Time returns the Time value associated with this Time.
func (t Time) Time() time.Time {
	return time.Time(t)
}

// Set implements flag.Value.
func (t *Time) Set(value string) error {
	timeValue, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return err
	}
	*t = Time(timeValue.UTC())
	return nil
}

func (t *Time) String() string {
	return time.Time(*t).String()
}

func (t Time) Equal(o Time) bool {
	return time.Time(t).Equal(time.Time(o))
}

// UnmarshalJSON implements json.Unmarshaler.
//
// Unmarshals a JSON entry into the underlying type. The entry is expected to
// contain a string corresponding to one of the enum's keys.
func (t *Time) UnmarshalJSON(data []byte) error {
	var value time.Time
	if err := value.UnmarshalJSON(data); err != nil {
		return err
	}
	*t = Time(value.UTC())
	return nil
}

// MarshalJSON implements json.Marshaler.
//
// Marshals a Time into an RFC3339 time string.
func (t Time) MarshalJSON() ([]byte, error) {
	return t.Time().UTC().MarshalJSON()
}
