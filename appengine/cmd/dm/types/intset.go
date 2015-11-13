// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"sort"
)

// U32s is a sortable slice of uint32.
type U32s []uint32

func (s U32s) Len() int           { return len(s) }
func (s U32s) Less(i, j int) bool { return s[i] < s[j] }
func (s U32s) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s U32s) idx(val uint32) int {
	return sort.Search(
		len(s), func(i int) bool { return s[i] >= val })
}

// Has performs a quick (O(ln N)) lookup for `val` in the slice. It assumes that
// the slice is sorted.
func (s U32s) Has(val uint32) bool {
	idx := s.idx(val)
	return idx < len(s) && s[idx] == val
}
