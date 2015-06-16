// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clockflag

import (
	"errors"
	"flag"
	"fmt"
	"time"
)

// Duration is a Flag- and JSON-compatible Duration value.
type Duration time.Duration

var _ flag.Value = (*Duration)(nil)

// Set implements flag.Value.
func (d *Duration) Set(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

func (d *Duration) String() string {
	return time.Duration(*d).String()
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
	// Strip off leading and trailing quotes.
	s := string(data)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return errors.New("Duration JSON must be a valid JSON string.")
	}
	return d.Set(s[1 : len(s)-1])
}

// MarshalJSON implements json.Marshaler.
//
// Marshals a Duration into a duration string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, time.Duration(d).String())), nil
}
