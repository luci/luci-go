// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"fmt"
)

const (
	// MaxTagKeySize is the maximum number of bytes allowed in a tag's key
	// parameter.
	MaxTagKeySize = 64

	// MaxTagValueSize is the maximum number of bytes allowed in a tag's value
	// parameter.
	MaxTagValueSize = 4096
)

// StreamTag is a stream tag key/value pair.
type StreamTag struct {
	// Key is the StreamTag's key property.
	//
	// A Key may be at most MaxTagKeySize characters, and has the same naming
	// restrictions as a StreamName.
	Key string
	// Value is the StreamTag's value property. It is not subject to any naming
	// limitations, and may be empty. It may be no longer than MaxTagValueSize.
	Value string
}

// Validate returns an error if the Tag contains a nonconfirming value.
func (t *StreamTag) Validate() error {
	if err := StreamName(t.Key).Validate(); err != nil {
		return fmt.Errorf("invalid tag key: %s", err)
	}
	if len(t.Key) > MaxTagKeySize {
		return fmt.Errorf("tag key exceeds maximum size (%d > %d)", len(t.Value), MaxTagKeySize)
	}
	if len(t.Value) > MaxTagValueSize {
		return fmt.Errorf("tag value exceeds maximum size (%d > %d)", len(t.Value), MaxTagValueSize)
	}
	return nil
}
