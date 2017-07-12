// Copyright 2017 The LUCI Authors.
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

package mathrand

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/rand"
)

// SeedRandomly seeds global math/rand RNG with bytes from crypto-random source.
//
// It is a good idea to call this in main() of some process to guarantee that
// multiple instances of this process don't go through the exact same PRNG
// sequence (go runtime doesn't seed math/rand itself).
func SeedRandomly() {
	var seed int64
	if err := binary.Read(cryptorand.Reader, binary.LittleEndian, &seed); err != nil {
		panic(err)
	}
	rand.Seed(seed)
}
