// Copyright 2018 The LUCI Authors.
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
	"regexp"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// durationRegex is the regular expression valid duration strings must match.
const durationRegex = "^(-?[0-9]+)(s|m|h|d|mo)$"

// parseDuration parses the given duration string and returns it in seconds.
// Duration must be in <int64><unit> form. Valid units are "s", "m", "h", "d", and "mo".
// "s" means seconds, "m" means minutes, "h" means hours, "d" means days, "mo" means months.
// 1 minute is 60 seconds, 1 hour is 60 minutes, 1 day is 24 hours, 1 month is 30 days.
func parseDuration(s string) (int64, error) {
	r, err := regexp.Compile(durationRegex)
	if err != nil {
		return 0, errors.Annotate(err, "invalid regex %q", durationRegex).Err()
	}
	// Decompose into a slice of [s, <int64>, <unit>].
	m := r.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, errors.Reason("duration did not match regex %q", durationRegex).Err()
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, errors.Annotate(err, "invalid int64").Err()
	}
	switch m[2] {
	case "s":
	case "m":
		n *= 60
	case "h":
		n *= 3600
	case "d":
		n *= 86400
	case "mo":
		n *= 2592000
	default:
		return 0, errors.Reason("invalid unit %q", m[2]).Err()
	}
	// Ignore multiplicative overflow. It turns n negative, but Validate checks this.
	return n, nil
}

// Normalize ensures this time period specifies seconds.
// Parses and converts duration to seconds if necessary.
func (t *TimePeriod) Normalize() error {
	if t.GetDuration() == "" {
		return nil
	}
	n, err := parseDuration(t.GetDuration())
	if err != nil {
		return errors.Annotate(err, "invalid duration").Err()
	}
	t.Time = &TimePeriod_Seconds{
		Seconds: n,
	}
	return nil
}

// Validate validates this time period.
func (t *TimePeriod) Validate(c *validation.Context) {
	switch {
	case t.GetDuration() == "" && t.GetSeconds() == 0:
		c.Errorf("duration or seconds is required")
	case t.GetDuration() != "":
		if err := t.Normalize(); err != nil {
			c.Errorf("%s", err)
		} else if t.GetSeconds() < 0 {
			c.Errorf("duration must be positive")
		}
	case t.GetSeconds() < 0:
		c.Errorf("seconds must be positive")
	}
}
