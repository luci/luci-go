// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"bytes"
)

// Join is a convenience invocation of bytes.Join(itms, nil)
func Join(itms ...[]byte) []byte {
	return bytes.Join(itms, nil)
}

// Invert simply inverts all the bytes in bs.
func Invert(bs []byte) []byte {
	if len(bs) == 0 {
		return nil
	}
	ret := make([]byte, len(bs))
	for i, b := range bs {
		ret[i] = 0xFF ^ b
	}
	return ret
}

// Increment attempts to increment a copy of bstr as if adding 1 to an integer.
//
// If it overflows, the returned []byte will be all 0's, and the overflow bool
// will be true.
func Increment(bstr []byte) ([]byte, bool) {
	ret := Join(bstr)
	for i := len(ret) - 1; i >= 0; i-- {
		if ret[i] == 0xFF {
			ret[i] = 0
		} else {
			ret[i]++
			return ret, false
		}
	}
	return ret, true
}
