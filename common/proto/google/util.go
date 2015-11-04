// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package google

import (
	"time"
)

const (
	nanosecondsInASecond = int64(time.Second / time.Nanosecond)
)

// NewTimestamp creates a new Timestamp protobuf from a time.Time type.
func NewTimestamp(t time.Time) *Timestamp {
	if t.IsZero() {
		return nil
	}

	nanos := t.UnixNano()
	return &Timestamp{
		Seconds: nanos / nanosecondsInASecond,
		Nanos:   int32(nanos % nanosecondsInASecond),
	}
}

// Time returns the time.Time associated with a Timestamp protobuf.
func (t *Timestamp) Time() time.Time {
	return time.Unix(t.Seconds, int64(t.Nanos)).UTC()
}

// NewDuration creates a new Duration protobuf from a time.Duration.
func NewDuration(d time.Duration) *Duration {
	if d == 0 {
		return nil
	}

	nanos := d.Nanoseconds()
	return &Duration{
		Seconds: nanos / nanosecondsInASecond,
		Nanos:   int32(nanos % nanosecondsInASecond),
	}
}

// Duration returns the time.Duration associated with a Duration protobuf.
func (d *Duration) Duration() time.Duration {
	return (time.Duration(d.Seconds) * time.Second) + (time.Duration(d.Nanos) * time.Nanosecond)
}
