// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolated

import (
	"encoding/hex"
	"hash"
	"io"
	"os"
)

// Sum is a shortcut to get a HexDigest from a hash.Hash.
func Sum(h hash.Hash) HexDigest {
	return HexDigest(hex.EncodeToString(h.Sum(nil)))
}

// Hash hashes a reader and returns a HexDigest from it.
func Hash(src io.Reader) (HexDigest, error) {
	h := GetHash()
	_, err := io.Copy(h, src)
	if err != nil {
		return HexDigest(""), err
	}
	return Sum(h), nil
}

// HashBytes hashes content and returns a HexDigest from it.
func HashBytes(content []byte) HexDigest {
	h := GetHash()
	_, _ = h.Write(content)
	return Sum(h)
}

// HashFile hashes a file and returns a DigestItem out of it.
func HashFile(path string) (DigestItem, error) {
	h := GetHash()
	f, err := os.Open(path)
	if err != nil {
		return DigestItem{}, err
	}
	defer f.Close()
	size, err := io.Copy(h, f)
	if err != nil {
		return DigestItem{}, err
	}
	return DigestItem{Digest: Sum(h), IsIsolated: false, Size: size}, nil
}
