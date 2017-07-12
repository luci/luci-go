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
	"fmt"
	"time"
)

// DurationHelp is generic Duration help.
const DurationHelp = "A duration string is a sequence of <number><unit> such as 2h15m. " +
	"Supported units are 'ns', 'us'/'µs', 'ms', 's', 'm', and 'h'."

// Duration is a Flag- and JSON-compatible Duration value.
type Duration time.Duration

var _ flag.Value = (*Duration)(nil)

// Set implements flag.Value.
func (d *Duration) Set(value string) (err error) {
	*d, err = ParseDuration(value)
	return
}

func (d *Duration) String() string {
	return FormatDuration(time.Duration(*d))
}

// IsZero tests if this Duration is the zero value.
func (d Duration) IsZero() bool {
	return time.Duration(d) == time.Duration(0)
}

// UnmarshalJSON implements json.Unmarshaler.
//
// Unmarshals a JSON entry into the underlying type. The entry is expected to
// contain a string corresponding to one of the enum's keys.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return d.Set(s)
}

// MarshalJSON implements json.Marshaler.
//
// Marshals a Duration into a duration string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, time.Duration(d).String())), nil
}

// ParseDuration parses a clockflag Duration from a string. This is basically
// a typed fall-through to time.ParseDuration.
func ParseDuration(v string) (Duration, error) {
	duration, err := time.ParseDuration(v)
	if err != nil {
		return 0, err
	}
	return Duration(duration), nil
}

// FormatDuration formats a time.Duration into a string that can be parsed with
// ParseDuration.
func FormatDuration(d time.Duration) string {
	return d.String()
}
