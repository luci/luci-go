// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

// memoryCorruption is a marker function to indicate that given error is
// actually due to corrupted memory to make it easier to read the code.
func memoryCorruption(err error) {
	if err != nil {
		panic(err)
	}
}

// impossible is a marker function to indicate that the given error is an
// impossible state, due to conditions outside of the function.
func impossible(err error) {
	if err != nil {
		panic(err)
	}
}
