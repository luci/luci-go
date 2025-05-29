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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// Normalize ensures this time period is in seconds.
// Parses and converts duration to seconds if necessary.
func (t *TimePeriod) Normalize() error {
	if t.GetDuration() == "" {
		return nil
	}
	n, err := t.Time.(*TimePeriod_Duration).ToSeconds()
	if err != nil {
		return errors.Fmt("invalid duration: %w", err)
	}
	t.Time = &TimePeriod_Seconds{
		Seconds: n,
	}
	return nil
}

// ToSeconds returns this time period in seconds. Clamps to math.MaxInt64.
func (t *TimePeriod) ToSeconds() (int64, error) {
	if t == nil {
		return 0, nil
	}
	switch t := t.Time.(type) {
	case *TimePeriod_Duration:
		return t.ToSeconds()
	case *TimePeriod_Seconds:
		return t.Seconds, nil
	default:
		return 0, errors.Fmt("unexpected type %T", t)
	}
}

// Validate validates this time period.
func (t *TimePeriod) Validate(c *validation.Context) {
	switch {
	case t.GetDuration() != "":
		c.Enter("duration")
		t.Time.(*TimePeriod_Duration).Validate(c)
		c.Exit()
	case t.GetSeconds() < 0:
		c.Errorf("seconds must be non-negative")
	}
}
