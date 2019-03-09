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
	"regexp"
	"strconv"
	"time"

	"google.golang.org/genproto/googleapis/type/dayofweek"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// timeRegex is the regular expression valid time strings must match.
const timeRegex = "^([0-2]?[0-9]):([0-6][0-9])$"

// ToTime returns this time of day relative to the current day.
func (t *TimeOfDay) ToTime() (time.Time, error) {
	now := time.Now()
	loc, err := time.LoadLocation(t.GetLocation())
	if err != nil {
		return now, errors.Reason("invalid location").Err()
	}
	now = now.In(loc)
	// Decompose the time into a slice of [time, <hour>, <minute>].
	m := regexp.MustCompile(timeRegex).FindStringSubmatch(t.GetTime())
	if len(m) != 3 {
		return now, errors.Reason("time must match regex %q", timeRegex).Err()
	}
	hr, err := strconv.Atoi(m[1])
	if err != nil || hr > 23 {
		return now, errors.Reason("time must not exceed 23:xx").Err()
	}
	min, err := strconv.Atoi(m[2])
	if err != nil || min > 59 {
		return now, errors.Reason("time must not exceed xx:59").Err()
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hr, min, 0, 0, loc), nil
}

// Validate validates this time of day.
func (t *TimeOfDay) Validate(c *validation.Context) {
	if t.GetDay() == dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED {
		c.Errorf("day must be specified")
	}
	_, err := t.ToTime()
	if err != nil {
		c.Errorf("%s", err)
	}
}
