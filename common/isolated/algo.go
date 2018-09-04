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

// Defines the algorithms used by the isolated server.

package isolated

import (
	"compress/zlib"
	"crypto/sha1"
	"hash"
	"io"
)

// GetHash returns a fresh instance of the hashing algorithm to be used to
// calculate the HexDigest.
//
// It is currently hardcoded to sha-1.
func GetHash() hash.Hash {
	return sha1.New()
}

// GetDecompressor returns a fresh instance of the decompression algorithm.
//
// It must be closed after use.
//
// It is currently hardcoded to RFC 1950 (zlib).
func GetDecompressor(in io.Reader) (io.ReadCloser, error) {
	return zlib.NewReader(in)
}

// GetCompressor returns a fresh instance of the compression algorithm.
//
// It must be closed after use.
//
// It is currently hardcoded to RFC 1950 (zlib).
func GetCompressor(out io.Writer) (io.WriteCloser, error) {
	// Higher compression level uses cpu resources more than we want.
	return zlib.NewWriterLevel(out, zlib.BestSpeed)
}

// HexDigest is the hash of a file that is hex-encoded. Only lower case letters
// are accepted.
type HexDigest string

// Validate returns true if the hash is valid.
func (d HexDigest) Validate() bool {
	if len(d) != sha1.Size*2 {
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
