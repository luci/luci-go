// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolateserver

import (
	"encoding/hex"
	"encoding/json"
	"hash"
	"strconv"
)

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

// Hash hashes content and returns a HexDigest from it.
func Hash(h hash.Hash, content []byte) HexDigest {
	h.Reset()
	h.Write(content)
	return HexDigest(hex.EncodeToString(h.Sum(nil)))
}

// Int is a JSON/Cloud Endpoints friendly int type that will correctly parse
// string encoded integers found in JSON encoded data.
type Int int

func (i *Int) UnmarshalJSON(p []byte) error {
	val := 0
	if err := json.Unmarshal(p, &val); err == nil {
		*i = Int(val)
		return err
	}
	s := ""
	if err := json.Unmarshal(p, &s); err != nil {
		return err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*i = Int(v)
	return nil
}
