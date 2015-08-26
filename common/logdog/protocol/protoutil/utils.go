// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package protoutil

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
)

const (
	nanosecondsInASecond = int64(time.Second / time.Nanosecond)
)

// DescriptorPath returns the StreamPath for a descriptor.
func DescriptorPath(d *protocol.LogStreamDescriptor) types.StreamPath {
	if d == nil {
		return types.StreamPath("")
	}
	return types.StreamName(d.GetPrefix()).Join(types.StreamName(d.GetName()))
}

// NewTimestamp creates a new Butler protocol Timestamp from a time.Time type.
func NewTimestamp(t time.Time) *protocol.Timestamp {
	val := t.UTC().Format(time.RFC3339Nano)
	return &protocol.Timestamp{
		Value: &val,
	}
}

// GetTimestampTime returns the time.Time associated with the Timestamp.
func GetTimestampTime(ts *protocol.Timestamp) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, ts.GetValue())
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp from [%s]: %s", ts.GetValue(), err)
	}
	return t.UTC(), nil
}

// NewTimeOffset returns the equivalent time offset in microseconds for a
// time.Duration.
func NewTimeOffset(d time.Duration) *protocol.TimeOffset {
	nano := d.Nanoseconds()
	if nano < 0 {
		nano = 0
	}
	seconds := nano / nanosecondsInASecond
	nanoseconds := nano - (seconds * nanosecondsInASecond)

	to := protocol.TimeOffset{}
	if seconds != 0 {
		u32Seconds := uint32(seconds)
		to.Seconds = &u32Seconds
	}
	if nanoseconds != 0 {
		u32Nanoseconds := uint32(nanoseconds)
		to.Nanoseconds = &u32Nanoseconds
	}
	return &to
}

// TimeOffset converts a TimeOffset protocol buffer to a time.Duration.
func TimeOffset(o *protocol.TimeOffset) time.Duration {
	return (time.Duration(o.GetSeconds()) * time.Second) + (time.Duration(o.GetNanoseconds()) * time.Nanosecond)
}

// ValidateDescriptor returns an error if the supplied LogStreamDescriptor is
// not complete and valid.
func ValidateDescriptor(d *protocol.LogStreamDescriptor) error {
	if err := types.StreamName(d.GetPrefix()).Validate(); err != nil {
		return fmt.Errorf("invalid prefix: %s", err)
	}
	if err := types.StreamName(d.GetName()).Validate(); err != nil {
		return fmt.Errorf("invalid name: %s", err)
	}

	timestamp := d.GetTimestamp()
	if timestamp == nil {
		return errors.New("missing timestamp")
	}
	if _, err := GetTimestampTime(timestamp); err != nil {
		return fmt.Errorf("invalid timestamp value [%s]: %s", timestamp.GetValue(), err)
	}
	for i, tag := range d.GetTags() {
		t := types.StreamTag{
			Key:   tag.GetKey(),
			Value: tag.GetValue(),
		}
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid tag #%d: %s", i, err)
		}
	}
	return nil
}

// ValidateLogEntry returns an error if the supplied LogEntry is not complete
// and valid.
func ValidateLogEntry(e *protocol.LogEntry) error {
	hasContent := len(e.GetLines()) > 0
	if !hasContent {
		for _, d := range e.GetData() {
			if len(d.Value) > 0 {
				hasContent = true
				break
			}
		}
	}
	if !hasContent {
		return errors.New("no content")
	}
	return nil
}
