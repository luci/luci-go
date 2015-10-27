// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package protocol

import (
	"errors"
	"fmt"

	"github.com/luci/luci-go/common/logdog/types"
)

var (
	// ErrNoContent indicates that a LogEntry has no data content.
	ErrNoContent = errors.New("no content")
)

// Validate returns an error if the supplied LogStreamDescriptor is not complete
// and valid.
//
// If prefix is true, the Prefix field will be validated; otherwise, it will
// be ignored. This can be useful when attempting to validate a
// LogStreamDescriptor before the application-assigned Prefix is known.
func (d *LogStreamDescriptor) Validate(prefix bool) error {
	if d == nil {
		return errors.New("descriptor is nil")
	}

	if prefix {
		if err := types.StreamName(d.Prefix).Validate(); err != nil {
			return fmt.Errorf("invalid prefix: %s", err)
		}
	}
	if err := types.StreamName(d.Name).Validate(); err != nil {
		return fmt.Errorf("invalid name: %s", err)
	}

	switch d.StreamType {
	case LogStreamDescriptor_TEXT, LogStreamDescriptor_BINARY, LogStreamDescriptor_DATAGRAM:
		break

	default:
		return fmt.Errorf("invalid stream type: %v", d.StreamType)
	}

	if d.ContentType == "" {
		return errors.New("missing content type")
	}

	if d.Timestamp == nil {
		return errors.New("missing timestamp")
	}
	for i, tag := range d.GetTags() {
		t := types.StreamTag{
			Key:   tag.Key,
			Value: tag.Value,
		}
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid tag #%d: %s", i, err)
		}
	}
	return nil
}

// Path returns a types.StreamPath constructed from the LogStreamDesciptor's
// Prefix and Name fields.
func (d *LogStreamDescriptor) Path() types.StreamPath {
	return types.StreamName(d.Prefix).Join(types.StreamName(d.Name))
}

// Validate checks a supplied LogEntry against its LogStreamDescriptor for
// validity, returning an error if it is not valid.
//
// If the LogEntry is otherwise valid, but has no content, ErrNoContent will be
// returned.
func (e *LogEntry) Validate(d *LogStreamDescriptor) error {
	if e == nil {
		return errors.New("entry is nil")
	}

	// Check for content.
	switch d.StreamType {
	case LogStreamDescriptor_TEXT:
		if t := e.GetText(); t == nil || len(t.Lines) == 0 {
			return ErrNoContent
		}

	case LogStreamDescriptor_BINARY:
		if b := e.GetBinary(); b == nil || len(b.Data) == 0 {
			return ErrNoContent
		}

	case LogStreamDescriptor_DATAGRAM:
		if d := e.GetDatagram(); d == nil {
			return ErrNoContent
		}
	}
	return nil
}
