// Copyright 2023 The LUCI Authors.
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

package base128

// DecodedLen returns the number of bytes `encLen` encoded bytes decodes to.
func DecodedLen(encLen int) int {
	return (encLen * 7) / 8
}

// EncodedLen returns the number of bytes that `dataLen` bytes will encode to.
func EncodedLen(dataLen int) int {
	return (((dataLen * 8) + 6) / 7)
}
