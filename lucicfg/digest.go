// Copyright 2020 The LUCI Authors.
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

package lucicfg

import (
	"crypto/sha256"
	"fmt"
	"hash"

	"go.starlark.net/starlark"
)

// See also //internal/digest.star.
func init() {
	declNative("hash_digest", func(call nativeCall) (starlark.Value, error) {
		var algo, blob starlark.String
		if err := call.unpack(1, &algo, &blob); err != nil {
			return nil, err
		}

		var h hash.Hash
		switch algo.GoString() {
		case "SHA256":
			h = sha256.New()
		default:
			return nil, fmt.Errorf("digest.bytes: unknown hashing algorithm %q", algo.GoString())
		}

		h.Write([]byte(blob))
		return starlark.String(h.Sum(nil)), nil
	})
}
