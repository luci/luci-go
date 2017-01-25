// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package google

import (
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
)

const (
	nanosecondsInASecond = int64(time.Second / time.Nanosecond)
)

// NewTimestamp creates a new Timestamp protobuf from a time.Time type.
func NewTimestamp(v time.Time) *timestamp.Timestamp {
	return LoadTimestamp(nil, v)
}

// LoadTimestamp replaces the value in the supplied Timestamp with the specified
// time.
//
// If the supplied Timestamp is nil and the time is non-zero, a new Timestamp
// will be generated. The populated Timestamp will be returned.
func LoadTimestamp(t *timestamp.Timestamp, v time.Time) *timestamp.Timestamp {
	if t == nil {
		if v.IsZero() {
			return nil
		}

		t = &timestamp.Timestamp{}
	}

	t.Seconds = v.Unix()
	t.Nanos = int32(v.Nanosecond())
	return t
}

// TimeFromProto returns the time.Time associated with a Timestamp protobuf.
func TimeFromProto(t *timestamp.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Unix(t.Seconds, int64(t.Nanos)).UTC()
}

// NewDuration creates a new Duration protobuf from a time.Duration.
func NewDuration(v time.Duration) *duration.Duration {
	return LoadDuration(nil, v)
}

// LoadDuration replaces the value in the supplied Duration with the specified
// value.
//
// If the supplied Duration is nil and the value is non-zero, a new Duration
// will be generated. The populated Duration will be returned.
func LoadDuration(d *duration.Duration, v time.Duration) *duration.Duration {
	if d == nil {
		if v == 0 {
			return nil
		}

		d = &duration.Duration{}
	}

	nanos := v.Nanoseconds()

	d.Seconds = nanos / nanosecondsInASecond
	d.Nanos = int32(nanos % nanosecondsInASecond)
	return d
}

// DurationFromProto returns the time.Duration associated with a Duration
// protobuf.
func DurationFromProto(d *duration.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return (time.Duration(d.Seconds) * time.Second) + (time.Duration(d.Nanos) * time.Nanosecond)
}
