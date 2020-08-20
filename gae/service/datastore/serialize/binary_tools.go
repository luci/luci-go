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
