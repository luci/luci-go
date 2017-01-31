// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
