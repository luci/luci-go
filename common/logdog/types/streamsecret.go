// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"crypto/rand"
	"errors"
)

const (
	// StreamSecretLength is the size, in bytes, of the stream secret.
	//
	// This value was chosen such that it is:
	// - Sufficiently large to avoid collisions.
	// - Can be expressed as a Base64 string without ugly padding.
	StreamSecretLength = 36
)

// StreamSecret is the stream secret value. This is a Base64-encoded secret
// value.
type StreamSecret []byte

// NewStreamSecret generates a new, default-length secret parameter.
func NewStreamSecret() (StreamSecret, error) {
	buf := make([]byte, StreamSecretLength)
	count, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}
	if count != len(buf) {
		return nil, errors.New("streamsecret: Generated secret with invalid size")
	}
	return StreamSecret(buf), nil
}
