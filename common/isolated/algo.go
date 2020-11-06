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
	"crypto"
	"io"
	"io/ioutil"
	"strings"
)

// GetHash returns a fresh instance of the hashing algorithm to be used to
// calculate the HexDigest.
//
// A prefix of "sha256-" and "sha512-" respectively returns a sha-256 and
// sha-512 instance. Otherwise a sha-1 instance is returned.
func GetHash(namespace string) crypto.Hash {
	if strings.HasPrefix(namespace, "sha256-") {
		return crypto.SHA256
	}
	if strings.HasPrefix(namespace, "sha512-") {
		return crypto.SHA512
	}
	return crypto.SHA1
}

// GetDecompressor returns a fresh instance of the decompression algorithm.
//
// It must be closed after use.
//
// It is currently hardcoded to RFC 1950 (zlib).
func GetDecompressor(namespace string, in io.Reader) (io.ReadCloser, error) {
	if isCompressed(namespace) {
		return zlib.NewReader(in)
	}
	return ioutil.NopCloser(in), nil
}

// GetCompressor returns a fresh instance of the compression algorithm.
//
// It must be closed after use.
//
// It is currently hardcoded to RFC 1950 (zlib).
func GetCompressor(namespace string, out io.Writer) (io.WriteCloser, error) {
	// Higher compression level uses cpu resources more than we want.
	if isCompressed(namespace) {
		return zlib.NewWriterLevel(out, zlib.BestSpeed)
	}
	return nopCloser{out}, nil
}

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

//

func isCompressed(namespace string) bool {
	return strings.HasSuffix(namespace, "-gzip") || strings.HasSuffix(namespace, "-flate")
}

type nopCloser struct {
	io.Writer
}

func (n nopCloser) Close() error {
	return nil
}
