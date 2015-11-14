// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// AttemptResult is the json display object for representing a DM AttemptResult.
type AttemptResult struct {
	ID types.AttemptID

	Data string `endpoints:"desc=The JSON result for this Attempt"`
}

// Less allows display attempt results to be compared based on their attempt
// id.
func (d *AttemptResult) Less(o *AttemptResult) bool {
	return d.ID.Less(&o.ID)
}

// AttemptResultSlice is an always-sorted list of display AttemptResults.
type AttemptResultSlice []*AttemptResult

func (s AttemptResultSlice) Len() int           { return len(s) }
func (s AttemptResultSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s AttemptResultSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get returns a pointer to the display attempt result in this slice identified
// by aid, or nil if it's not present. This takes O(ln N).
func (s AttemptResultSlice) Get(aid *types.AttemptID) *AttemptResult {
	idx := sort.Search(len(s), func(i int) bool {
		return !s[i].ID.Less(aid)
	})
	if idx < len(s) && s[idx].ID == *aid {
		return s[idx]
	}
	return nil
}

// Merge efficiently inserts the display AttemptResult into the
// AttemptResultSlice, if it's not present. This takes O(ln N) if a is already
// present, or O(N ln N) if it's not. Merge returns the inserted element, or nil
// if no element was inserted.
func (s *AttemptResultSlice) Merge(a *AttemptResult) *AttemptResult {
	if a == nil {
		return nil
	}
	cur := s.Get(&a.ID)
	if cur == nil {
		*s = append(*s, a)
		if len(*s) > 1 && a.Less((*s)[len(*s)-2]) {
			sort.Sort(*s)
		}
		return a
	}
	return nil
}
