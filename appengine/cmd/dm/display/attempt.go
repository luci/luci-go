// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"time"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// Attempt is the json display object for representing a DM Attempt.
type Attempt struct {
	ID types.AttemptID

	NumExecutions  uint32             `endpoints:"desc=The number of executions this Attempt has had."`
	State          types.AttemptState `endpoints:"desc=The current state of this Attempt"`
	Expiration     time.Time          `endpoints:"desc=The time at which this result will become Expired. Only set for attempts in the Finished or Expired states"`
	NumWaitingDeps uint32             `endpoints:"desc=The number of dependencies that this Attempt is blocked on. Only valid for attempts in the AddingDeps or Blocked states"`
}

// Less allows display attempts to be compared based on their ID.
func (d *Attempt) Less(o *Attempt) bool {
	return d.ID.Less(&o.ID)
}

// AttemptSlice is an always-sorted list of display Attempts.
type AttemptSlice []*Attempt

func (s AttemptSlice) Len() int           { return len(s) }
func (s AttemptSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s AttemptSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get returns a pointer to the display attempt in this slice identified by
// aid, or nil if it's not present. This takes O(ln N).
func (s AttemptSlice) Get(aid *types.AttemptID) *Attempt {
	idx := sort.Search(len(s), func(i int) bool {
		return !s[i].ID.Less(aid)
	})
	if idx < len(s) && s[idx].ID == *aid {
		return s[idx]
	}
	return nil
}

// Merge efficiently inserts the display Attempt into the AttemptSlice, if it's
// not present. This takes O(ln N) if a is already present, or O(N ln N) if
// it's not. Merge returns the inserted element, or nil if no element was
// inserted.
func (s *AttemptSlice) Merge(a *Attempt) *Attempt {
	if a == nil {
		return nil
	}
	cur := s.Get(&a.ID)
	if cur == nil {
		*s = append(*s, a)
		if len(*s) > 1 && a.Less(((*s)[len(*s)-2])) {
			sort.Sort(*s)
		}
		return a
	}
	return nil
}
