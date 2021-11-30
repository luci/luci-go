// Copyright 2021 The LUCI Authors.
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

package cache

import (
	"crypto"
	"encoding/hex"
	"hash"
	"io"
)

// HexDigest is the hash of a file that is hex-encoded. Only lower case letters
// are accepted.
type HexDigest string

// Validate returns true if the hash is valid.
func (d HexDigest) Validate(h crypto.Hash) bool {
	if l := h.Size() * 2; len(d) != l {
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

// HexDigests is a slice of HexDigest that implements sort.Interface.
type HexDigests []HexDigest

func (h HexDigests) Len() int           { return len(h) }
func (h HexDigests) Less(i, j int) bool { return h[i] < h[j] }
func (h HexDigests) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// Sum is a shortcut to get a HexDigest from a hash.Hash.
func Sum(h hash.Hash) HexDigest {
	return HexDigest(hex.EncodeToString(h.Sum(nil)))
}

// Hash hashes a reader and returns a HexDigest from it.
func Hash(h crypto.Hash, src io.Reader) (HexDigest, error) {
	a := h.New()
	_, err := io.Copy(a, src)
	if err != nil {
		return "", err
	}
	return Sum(a), nil
}

// HashBytes hashes content and returns a HexDigest from it.
func HashBytes(h crypto.Hash, content []byte) HexDigest {
	a := h.New()
	_, _ = a.Write(content)
	return Sum(a)
}
