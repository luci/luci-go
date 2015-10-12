// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package protoutil

import (
	"errors"
	"fmt"

	p "github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
)

var (
	// ErrNoContent indicates that a LogEntry has no data content.
	ErrNoContent = errors.New("no content")
)

// ValidateDescriptor returns an error if the supplied LogStreamDescriptor is
// not complete and valid.
func ValidateDescriptor(d *p.LogStreamDescriptor) error {
	if err := types.StreamName(d.Prefix).Validate(); err != nil {
		return fmt.Errorf("invalid prefix: %s", err)
	}
	if err := types.StreamName(d.Name).Validate(); err != nil {
		return fmt.Errorf("invalid name: %s", err)
	}

	switch d.StreamType {
	case p.LogStreamDescriptor_TEXT:
		if d.TextSource == nil {
			return errors.New("text stream is missing TextSource information")
		}

	case p.LogStreamDescriptor_BINARY:
		break
	case p.LogStreamDescriptor_DATAGRAM:
		break

	default:
		return fmt.Errorf("invalid stream type: %d", d.StreamType)
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

// ValidateLogEntry checks a supplied LogEntry against its LogStreamDescriptor
// for validity, returning an error if it is not valid.
//
// If the LogEntry is otherwise valid, but has no content, ErrNoContent will be
// returned.
func ValidateLogEntry(e *p.LogEntry, d *p.LogStreamDescriptor) error {
	// Check for content.
	switch d.StreamType {
	case p.LogStreamDescriptor_TEXT:
		if e.Text == nil || len(e.Text.Lines) == 0 {
			return ErrNoContent
		}

	case p.LogStreamDescriptor_BINARY:
		if e.Binary == nil || len(e.Binary.Data) == 0 {
			return ErrNoContent
		}

	case p.LogStreamDescriptor_DATAGRAM:
		if e.Datagram == nil {
			return ErrNoContent
		}
	}
	return nil
}
