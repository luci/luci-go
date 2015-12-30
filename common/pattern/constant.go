// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pattern

type constant bool

func (c constant) Match(s string) bool {
	return bool(c)
}

func (c constant) String() string {
	if c {
		return "*"
	}
	return ""
}

var (
	// Any matches anything.
	Any Pattern
	// None matches nothing.
	None Pattern
)

func init() {
	Any = constant(true)
	None = constant(false)
}
