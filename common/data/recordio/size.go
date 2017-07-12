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

package recordio

// FrameHeaderSize calculates the size of the RecordIO frame header for a given
// amount of data.
//
// A RecordIO frame header is a Uvarint containing the length. Uvarint values
// contain 7 bits of size data and 1 continuation bit (see encoding/binary).
func FrameHeaderSize(s int64) int {
	size := 1
	for s >>= 7; s > 0; s >>= 7 {
		size++
	}
	return size
}
