// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/xtgo/set"
)

// QuestAttempts represents zero or more AttemptIDs for a single Quest.
type QuestAttempts struct {
	QuestID  string     `endpoints:"desc=The Quest that this dependency points to"`
	Attempts types.U32s `endpoints:"desc=The Attempt(s) that this dependency points to"`
}

// Less compares QusetAttempts based on their QuestID.
func (d *QuestAttempts) Less(o *QuestAttempts) bool {
	return d.QuestID < o.QuestID
}

// QuestAttemptsSlice is an always-sorted slice of QuestAttempts objects.
type QuestAttemptsSlice []*QuestAttempts

func (s QuestAttemptsSlice) Len() int           { return len(s) }
func (s QuestAttemptsSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s QuestAttemptsSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get returns a single QuestAttempts from this slice, if it exists. Otherwise
// this returns nil.
func (s QuestAttemptsSlice) Get(qid string) *QuestAttempts {
	idx := sort.Search(len(s), func(i int) bool {
		return s[i].QuestID >= qid
	})
	if idx < len(s) && s[idx].QuestID == qid {
		return s[idx]
	}
	return nil
}

// Merge combines the provided QuestAttempts into this slice, possibly updating
// an existing QuestAttempts object in this slice if there is already data
// for that quest in the slice. This returns a new QuestAttempts object
// representing all of the added attempts. If no new attempts were added, this
// returns nil.
func (s *QuestAttemptsSlice) Merge(d *QuestAttempts) *QuestAttempts {
	if d == nil {
		return nil
	}
	cur := s.Get(d.QuestID)
	if cur == nil {
		*s = append(*s, d)
		if len(*s) > 1 && d.Less(((*s)[len(*s)-2])) {
			sort.Sort(*s)
		}
		return d
	}

	ret := *d
	ret.Attempts = make(types.U32s, len(d.Attempts), len(d.Attempts)+len(cur.Attempts))
	copy(ret.Attempts, d.Attempts)
	ret.Attempts = append(ret.Attempts, cur.Attempts...)
	ret.Attempts = ret.Attempts[:set.Diff(ret.Attempts, len(d.Attempts))]
	if len(ret.Attempts) == 0 {
		return nil
	}

	cur.Attempts = append(cur.Attempts, ret.Attempts...)
	sort.Sort(cur.Attempts)
	return &ret
}
