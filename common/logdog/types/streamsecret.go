// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"crypto/rand"
	"fmt"
)

const (
	// PrefixSecretLength is the size, in bytes, of the stream secret.
	//
	// This value was chosen such that it is:
	// - Sufficiently large to avoid collisions.
	// - Can be expressed as a Base64 string without ugly padding.
	PrefixSecretLength = 36
)

// PrefixSecret is the stream secret value. This is a Base64-encoded secret
// value.
type PrefixSecret []byte

// NewPrefixSecret generates a new, default-length secret parameter.
func NewPrefixSecret() (PrefixSecret, error) {
	buf := make([]byte, PrefixSecretLength)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}

	value := PrefixSecret(buf)
	if err := value.Validate(); err != nil {
		panic(err)
	}
	return value, nil
}

// Validate confirms that this prefix secret is conformant.
//
// Note that this does not scan the byte contents of the secret for any
// security-related parameters.
func (s PrefixSecret) Validate() error {
	if len(s) != PrefixSecretLength {
		return fmt.Errorf("invalid prefix secret length (%d != %d)", len(s), PrefixSecretLength)
	}
	return nil
}
