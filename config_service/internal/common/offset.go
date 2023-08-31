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

package common

import (
	"hash/fnv"
	"time"
)

// DistributeOffset deterministically chooses an offset across keys
// in [1..interval) range aiming for uniform distribution across all keys.
//
// A key is simply a concatenation of key parts with '\0' filler in between.
//
// Shamelessly copied from:
// https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/cv/internal/common/project_distribuition.go
// TODO(yiwzhang): Consider move to a common packages in luci-go.
func DistributeOffset(interval time.Duration, keyParts ...string) time.Duration {
	// Basic idea: interval/N*random(0..N), but deterministic on keyparts.
	// Use fast hash function, as we don't need strong collision resistance.
	h := fnv.New32a()
	for _, part := range keyParts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	r := h.Sum32()

	i := int64(interval)
	// Avoid losing precision for low pollInterval values by first shifting them
	// the more significant bits.
	shifted := 0
	for i < (int64(1) << 55) {
		i = i << 7
		shifted += 7
	}
	// i = i / N * r, where N = 2^32 since r is (0..2^32-1).
	i = (i >> 32) * int64(r)
	return time.Duration(i >> shifted)
}
