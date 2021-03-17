// Copyright 2015 The LUCI Authors.
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

package google

import (
	"time"

	durationpb "github.com/golang/protobuf/ptypes/duration"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
)

const (
	nanosecondsInASecond = int64(time.Second / time.Nanosecond)
)

// NewTimestamp creates a new Timestamp protobuf from a time.Time type.
func NewTimestamp(v time.Time) *timestamppb.Timestamp {
	return LoadTimestamp(nil, v)
}

// LoadTimestamp replaces the value in the supplied Timestamp with the specified
// time.
//
// If the supplied Timestamp is nil and the time is non-zero, a new Timestamp
// will be generated. The populated Timestamp will be returned.
func LoadTimestamp(t *timestamppb.Timestamp, v time.Time) *timestamppb.Timestamp {
	if t == nil {
		if v.IsZero() {
			return nil
		}

		t = &timestamppb.Timestamp{}
	}

	t.Seconds = v.Unix()
	t.Nanos = int32(v.Nanosecond())
	return t
}

// TimeFromProto returns the time.Time associated with a Timestamp protobuf.
func TimeFromProto(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	if t.Seconds == 0 && t.Nanos == 0 {
		// This is year 0000 as opposed to time.Unix(0, 0).UTC(), which is 1970.
		return time.Time{}
	}
	return time.Unix(t.Seconds, int64(t.Nanos)).UTC()
}

// NewDuration creates a new Duration protobuf from a time.Duration.
func NewDuration(v time.Duration) *durationpb.Duration {
	return LoadDuration(nil, v)
}

// LoadDuration replaces the value in the supplied Duration with the specified
// value.
//
// If the supplied Duration is nil and the value is non-zero, a new Duration
// will be generated. The populated Duration will be returned.
func LoadDuration(d *durationpb.Duration, v time.Duration) *durationpb.Duration {
	if d == nil {
		if v == 0 {
			return nil
		}

		d = &durationpb.Duration{}
	}

	nanos := v.Nanoseconds()

	d.Seconds = nanos / nanosecondsInASecond
	d.Nanos = int32(nanos % nanosecondsInASecond)
	return d
}

// DurationFromProto returns the time.Duration associated with a Duration
// protobuf.
func DurationFromProto(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return (time.Duration(d.Seconds) * time.Second) + (time.Duration(d.Nanos) * time.Nanosecond)
}
