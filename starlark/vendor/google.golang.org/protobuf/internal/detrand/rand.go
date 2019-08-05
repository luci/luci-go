// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package detrand provides deterministically random functionality.
//
// The pseudo-randomness of these functions is seeded by the program binary
// itself and guarantees that the output does not change within a program,
// while ensuring that the output is unstable across different builds.
package detrand

// Disable disables detrand such that all functions returns the zero value.
// This function is not concurrent-safe and must be called during program init.
func Disable() {
}

// Bool returns a deterministically random boolean.
func Bool() bool {
	return false
}

// Intn returns a deterministically random integer within [0,n).
func Intn(n int) int {
	return 0
}
