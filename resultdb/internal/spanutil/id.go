// Copyright 2025 The LUCI Authors.
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

package spanutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// hashPrefixBytes is the number of bytes of sha256 to prepend to a PK
// to achieve even distribution.
const hashPrefixBytes = 4

func PrefixWithHash(s string) string {
	h := sha256.Sum256([]byte(s))
	prefix := hex.EncodeToString(h[:hashPrefixBytes])
	return fmt.Sprintf("%s:%s", prefix, s)
}

func StripHashPrefix(s string) string {
	expectedPrefixLen := hex.EncodedLen(hashPrefixBytes) + 1 // +1 for separator
	if len(s) < expectedPrefixLen {
		panic(fmt.Sprintf("%q is too short", s))
	}
	return s[expectedPrefixLen:]
}
