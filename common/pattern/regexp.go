// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pattern

import "regexp"

type regexpMatch struct {
	r *regexp.Regexp
}

func (m regexpMatch) Match(s string) bool {
	return m.r.MatchString(s)
}

func (m regexpMatch) String() string {
	return "regex:" + m.r.String()
}

// Regexp returns a regular expression-based pattern.
func Regexp(expr *regexp.Regexp) Pattern {
	return regexpMatch{expr}
}
