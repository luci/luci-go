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

package coordinator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// HashID is a hex-encoded SHA256 hash.
type HashID string

const validHashIDChars = "0123456789abcdef"

func makeHashID(v string) HashID {
	hash := sha256.Sum256([]byte(v))
	return HashID(hex.EncodeToString(hash[:]))
}

// Normalize normalizes the hash ID and verifies its integrity.
func (id *HashID) Normalize() error {
	// encoding/hex encodes using lower-case hexadecimal. Note that this is a
	// no-op if the ID is already lowercase.
	idv := strings.ToLower(string(*id))

	if decodeSize := hex.DecodedLen(len(idv)); decodeSize != sha256.Size {
		return fmt.Errorf("invalid SHA256 hash size (%d != %d)", decodeSize, sha256.Size)
	}
	for i, r := range idv {
		if !strings.ContainsRune(validHashIDChars, r) {
			return fmt.Errorf("invalid character '%c' at %d", r, i)
		}
	}
	*id = HashID(idv)
	return nil
}
