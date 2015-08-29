// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

type stringSet map[string]struct{}

func (s stringSet) has(value string) bool {
	_, ret := s[value]
	return ret
}

// add adds to the set and returns true iff the value was not there before (i.e.
// it returns true if add actually modifies the stringSet).
func (s stringSet) add(value string) bool {
	ret := !s.has(value)
	s[value] = struct{}{}
	return ret
}

func (s stringSet) rm(value string) {
	delete(s, value)
}

func (s stringSet) dup() stringSet {
	ret := make(stringSet, len(s))
	for v := range s {
		ret.add(v)
	}
	return ret
}

func (s stringSet) getOne() string {
	for k := range s {
		return k
	}
	return ""
}
