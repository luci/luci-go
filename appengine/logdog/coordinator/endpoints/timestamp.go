// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package endpoints

import (
	"time"
)

// ToRFC3339 converts a time.Time to an RFC3339 timestamp string.
func ToRFC3339(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

// ParseRFC3339 parses an RFC3339 string into a time.Time.
func ParseRFC3339(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
