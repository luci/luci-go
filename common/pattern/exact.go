// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pattern

type exactMatch string

func (m exactMatch) Match(s string) bool {
	return string(m) == s
}

func (m exactMatch) String() string {
	return "exact:" + string(m)
}

// Exact returns a pattern that matches s only.
func Exact(s string) Pattern {
	if s == "" {
		return None
	}
	return exactMatch(s)
}
