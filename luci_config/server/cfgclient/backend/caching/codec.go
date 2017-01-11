// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package caching

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strings"

	"github.com/luci/luci-go/common/errors"
)

// Encode is a convenience method for generating a ZLIB-compressed JSON-encoded
// object.
func Encode(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	enc := json.NewEncoder(zw)

	if err := enc.Encode(v); err != nil {
		zw.Close()
		return nil, errors.Annotate(err).Reason("failed to JSON-encode").Err()
	}
	if err := zw.Close(); err != nil {
		return nil, errors.Annotate(err).Reason("failed to Close zlib Writer").Err()
	}
	return buf.Bytes(), nil
}

// Decode is a convenience method for decoding a ZLIB-compressed JSON-encoded
// object encoded by Encode.
func Decode(d []byte, v interface{}) error {
	zr, err := zlib.NewReader(bytes.NewReader(d))
	if err != nil {
		return errors.Annotate(err).Reason("failed to create zlib Reader").Err()
	}
	defer zr.Close()

	if err := json.NewDecoder(zr).Decode(v); err != nil {
		return errors.Annotate(err).Reason("failed to JSON-decode").Err()
	}
	return nil
}

// HashParams is a convenience method for hashing a series of strings into a
// unique hash key for that series.
//
// The output is fixed-size sha256.Size (32) bytes.
func HashParams(params ...string) []byte {
	size := 0
	for i := range params {
		size += len(params[i]) + 1
	}
	size += binary.MaxVarintLen64 + 1

	var buf bytes.Buffer
	buf.Grow(size)

	// Prefix the hash input with the number of elements. This will prevent some
	// inputs that include "0x00" from from stomping over other inputs.
	sizeBuf := [binary.MaxVarintLen64]byte{}
	_, _ = buf.Write(sizeBuf[:binary.PutUvarint(sizeBuf[:], uint64(len(params)))])
	buf.WriteByte(0x00)

	// Write each string to the Buffer.
	for i, s := range params {
		// First, scan through the string to see if it has any "0x00". It probably
		// won't! But if it does, it could create a conflict. If we observe this,
		// we'll escape "0x00".
		if idx := strings.Index(s, "\x00"); idx >= 0 {
			_, _ = buf.Write(bytes.Replace([]byte(params[i]), []byte{0x00}, []byte{0x00, 0x00}, -1))
		} else {
			_, _ = buf.WriteString(s)
		}
		buf.WriteByte(0x00)
	}

	hash := sha256.Sum256(buf.Bytes())
	return hash[:]
}
