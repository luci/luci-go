// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
)

func bjoin(itms ...[]byte) []byte {
	total := 0
	for _, i := range itms {
		total += len(i)
	}
	ret := make([]byte, 0, total)
	for _, i := range itms {
		ret = append(ret, i...)
	}
	return ret
}

// invert simply inverts all the bytes in bs.
func invert(bs []byte) []byte {
	if len(bs) == 0 {
		return nil
	}
	ret := make([]byte, len(bs))
	for i, b := range bs {
		ret[i] = 0xFF ^ b
	}
	return ret
}

func increment(bstr []byte) []byte {
	if len(bstr) > 0 {
		// Copy bstr
		ret := bjoin(bstr)
		for i := len(ret) - 1; i >= 0; i-- {
			if ret[i] == 0xFF {
				ret[i] = 0
			} else {
				ret[i]++
				return ret
			}
		}
	}

	// This byte string was ALL 0xFF's. The only safe incrementation to do here
	// would be to add a new byte to the beginning of bstr with the value 0x01,
	// and a byte to the beginning OF ALL OTHER []byte's which bstr may be
	// compared with. This is obviously impossible to do here, so panic. If we
	// hit this, then we would need to add a spare 0 byte before every index
	// column.
	//
	// Another way to think about this is that we just accumulated a 'carry' bit,
	// and the new value has overflowed this representation.
	//
	// Fortunately, the first byte of a serialized index column entry is a
	// PropertyType byte, and the only valid values that we'll be incrementing
	// are never equal to 0xFF, since they have the high bit set (so either they're
	// 0x8*, or 0x7*, depending on if it's inverted).
	impossible(fmt.Errorf("incrementing %v would require more sigfigs", bstr))
	return nil
}
