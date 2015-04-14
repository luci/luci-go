// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolateserver

import "hash"

// Digest is the hash of a file.
type Digest []byte

// HexDigest is the hash of a file that is hex-encoded. Only lower case letters
// are accepted.
type HexDigest string

// Validate returns true if the hash is valid.
func (d HexDigest) Validate(h hash.Hash) bool {
	if len(d) != h.Size()*2 {
		return false
	}
	for _, c := range d {
		if ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
