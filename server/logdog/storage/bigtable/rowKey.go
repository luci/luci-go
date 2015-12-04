// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/luci/luci-go/common/cmpbin"
)

// rowKeyBufferPool stores a pool of allocated Buffer instances for reuse when
// constructing row keys.
var (
	// errMalformedRowKey is an error that is returned if the row key in the
	// tables does not comform to our row key structure.
	errMalformedRowKey = errors.New("bigtable: malformed row key")

	// encodedPrefixSize is the size in bytes of the encoded row key prefix. All
	// rows from the same stream path share this prefix.
	encodedPrefixSize = base64.URLEncoding.EncodedLen(sha256.Size) + len("~")
	// maxEncodedKeySize is the maximum size in bytes of a full row key.
	maxEncodedKeySize = encodedPrefixSize + hex.EncodedLen(cmpbin.MaxIntLen64)

	rowKeyBufferPool = sync.Pool{
		New: func() interface{} {
			return &rowKeyBuffers{}
		},
	}
)

type rowKeyBuffers struct {
	// binBuf is a Buffer to write binary data for encoding.
	binBuf bytes.Buffer
	// key is where the encoded key get built.
	key []byte
}

func (rkb *rowKeyBuffers) reset() {
	rkb.binBuf.Reset()
	if rkb.key == nil {
		rkb.key = make([]byte, maxEncodedKeySize)
	}
}

// rowKey is a BigTable row key.
//
// The row key is formed from a Path and its Index. The goal:
// - Rows with the same path should be clustered.
// - Rows with the same path should be sorted according to index.
//
// Since BigTable rows must be valid UTF8, and since paths are effectively
// unbounded, the row key will be formed by composing:
// [ base64(sha256(path)) ] + '~' + [ hex(cmpbin(index)) ]
type rowKey struct {
	pathHash []byte
	index    int64
}

// newRowKey generates the row key matching a given entry path and index.
func newRowKey(path string, index int64) *rowKey {
	pathHash := sha256.Sum256([]byte(path))
	return &rowKey{
		pathHash: pathHash[:],
		index:    index,
	}
}

// decodeRowKey decodes an encoded row key into its structural components.
func decodeRowKey(v string) (*rowKey, error) {
	keyParts := strings.SplitN(v, "~", 2)
	if len(keyParts) != 2 {
		return nil, errMalformedRowKey
	}

	hashEnc, idxEnc := keyParts[0], keyParts[1]
	if base64.URLEncoding.DecodedLen(len(hashEnc)) < sha256.Size {
		return nil, errMalformedRowKey
	}

	// Decode encoded path hash.
	var err error
	rk := rowKey{}
	rk.pathHash, err = base64.URLEncoding.DecodeString(hashEnc)
	if err != nil {
		return nil, errMalformedRowKey
	}

	// Decode index.
	idxBytes, err := hex.DecodeString(idxEnc)
	if err != nil {
		return nil, errMalformedRowKey
	}

	dr := bytes.NewReader(idxBytes)
	index, _, err := cmpbin.ReadInt(dr)
	if err != nil {
		return nil, errMalformedRowKey
	}
	rk.index = index

	// There should be no more data.
	if dr.Len() > 0 {
		return nil, errMalformedRowKey
	}
	return &rk, nil
}

func (rk *rowKey) String() string {
	return rk.encode()
}

// newRowKey instantiates a new rowKey from its components.
func (rk *rowKey) encode() string {
	rkb := rowKeyBufferPool.Get().(*rowKeyBuffers)
	defer rowKeyBufferPool.Put(rkb)
	rkb.reset()

	// Encode index to "cmpbin".
	cmpbin.WriteInt(&rkb.binBuf, rk.index)

	// Write the final key to "key": (base64(HASH)~hex(INDEX))
	l := 0
	base64.URLEncoding.Encode(rkb.key[l:], rk.pathHash)
	l += base64.URLEncoding.EncodedLen(len(rk.pathHash))
	l += utf8.EncodeRune(rkb.key[l:], '~')
	l += hex.Encode(rkb.key[l:], rkb.binBuf.Bytes())

	return string(rkb.key[:l])
}

// prefix returns the encoded path prefix for the row key.
func (rk *rowKey) pathPrefix() string {
	return rk.encode()[:encodedPrefixSize]
}

// sharesPrefixWith tests if the "path" component of the row key "rk" matches
// the "path" component of "o".
func (rk *rowKey) sharesPathWith(o *rowKey) bool {
	return bytes.Equal(rk.pathHash, o.pathHash)
}
