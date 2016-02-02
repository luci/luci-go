// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"fmt"
	"html/template"
	"strings"
)

// A collection of useful templating functions

// funcMap is what gets fed into the template bundle.
var funcMap = template.FuncMap{
	"humanDuration": humanDuration,
	"humanTimeRFC":  humanTimeRFC,
	"startswith":    strings.HasPrefix,
}

// humanDuration takes a time t in seconds as a duration and translates it
// into a human readable string of x units y units, where x and y could be in
// days, hours, minutes, or seconds, whichever is the largest.
func humanDuration(t uint64) string {
	// Input: Duration in seconds.  Output, the duration pretty printed.
	day := t / 86400
	hr := (t % 86400) / 3600
	min := (t % 3600) / 60
	sec := t % 60

	if day > 0 {
		if hr != 0 {
			return fmt.Sprintf("%d days %d hrs", day, hr)
		}
		return fmt.Sprintf("%d days", day)
	} else if hr > 0 {
		if min != 0 {
			return fmt.Sprintf("%d hrs %d mins", hr, min)
		}
		return fmt.Sprintf("%d hrs", hr)
	} else {
		if min > 0 {
			if sec != 0 {
				return fmt.Sprintf("%d mins %d secs", min, sec)
			}
			return fmt.Sprintf("%d mins", min)
		}
		return fmt.Sprintf("%d secs", sec)
	}
}

// humanTimeRFC takes in the time represented as a RFC3339 string and returns
// something more human readable.
func humanTimeRFC(s string) string {
	// TODO(hinoka): Implement me.
	return s
}
