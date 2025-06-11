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

package config

import (
	"math"
	"regexp"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// durationRegex is the regular expression valid duration strings must match.
const durationRegex = "^([0-9]+)(s|m|h|d|mo)$"

// ToSeconds returns this duration in seconds. Clamps to math.MaxInt64.
func (d *TimePeriod_Duration) ToSeconds() (int64, error) {
	// Decompose the duration into a slice of [duration, <int64>, <unit>].
	m := regexp.MustCompile(durationRegex).FindStringSubmatch(d.Duration)
	if len(m) != 3 {
		return 0, errors.Fmt("duration must match regex %q", durationRegex)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, errors.Fmt("duration must not exceed %q", int64(math.MaxInt64))
	}
	var mul int64
	switch m[2] {
	case "s":
		// 1 second.
		mul = 1
	case "m":
		// 60 seconds.
		mul = 60
	case "h":
		// 60 minutes.
		mul = 3600
	case "d":
		// 24 hours.
		mul = 86400
	case "mo":
		// 30 days.
		mul = 2592000
	}
	if n > math.MaxInt64/mul {
		n = math.MaxInt64
	} else {
		n *= mul
	}
	return n, nil
}

// Validate validates this duration.
func (d *TimePeriod_Duration) Validate(c *validation.Context) {
	if _, err := d.ToSeconds(); err != nil {
		c.Errorf("%s", err)
	}
}
