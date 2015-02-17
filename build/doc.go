// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package build provides information about environment where binary was build.
*/

package build

// InfoString returns a string that can be put into CLI application help output
// to identify how the executable was built.
func InfoString() string {
	if ReleaseBuild {
		return ""
	}
	return "(testing build)"
}
