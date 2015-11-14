// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// DepsFromAttempt is the json display structure that represents one or more
// dependencies from a single Attempt to zero or more other Attempts.
type DepsFromAttempt struct {
	From types.AttemptID `endpoints:"desc=The Quest that this dependency points from"`
	To   QuestAttemptsSlice
}

// Less compares Deps based on their `From` AttemptID.
func (d *DepsFromAttempt) Less(o *DepsFromAttempt) bool {
	return d.From.Less(&o.From)
}

// DepsFromAttemptSlice is an always-sorted slice of Deps.
type DepsFromAttemptSlice []*DepsFromAttempt

func (s DepsFromAttemptSlice) Len() int           { return len(s) }
func (s DepsFromAttemptSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s DepsFromAttemptSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get returns a single DepsFromAttempt from a DepsFromAttemptSlice, if it exists.
func (s DepsFromAttemptSlice) Get(aid *types.AttemptID) *DepsFromAttempt {
	idx := sort.Search(len(s), func(i int) bool {
		return !s[i].From.Less(aid)
	})
	if idx < len(s) && s[idx].From == *aid {
		return s[idx]
	}
	return nil
}

// Merge merges the dependencies represented by `d` into this
// DepsFromAttemptSlice. This may require updating an existing DepsFromAttempt
// object in this slice. It will return a new DepsFromAttempt object which
// contains only the DepsFromAttempt edges that were merged into this slice, or
// nil if no new edges were added.
func (s *DepsFromAttemptSlice) Merge(d *DepsFromAttempt) *DepsFromAttempt {
	if d == nil {
		return nil
	}
	cur := s.Get(&d.From)
	if cur == nil {
		// add
		*s = append(*s, d)
		if len(*s) > 1 && d.Less(((*s)[len(*s)-2])) {
			sort.Sort(*s)
		}
		return d
	}

	// merge
	ret := *d
	ret.To = nil
	for _, d2q := range d.To {
		ret.To.Merge(cur.To.Merge(d2q))
	}
	if len(ret.To) == 0 {
		return nil
	}
	return &ret
}
