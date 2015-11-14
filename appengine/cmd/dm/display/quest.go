// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"time"
)

// Quest is the json display object for a DM Quest.
type Quest struct {
	ID          string    `endpoints:"desc=The Quest ID"`
	Payload     string    `endpoints:"desc=The Quest JSON payload for the distributor"`
	Distributor string    `endpoints:"desc=The Distributor to use for this Quest"`
	Created     time.Time `endpoints:"desc=The time that this quest was created"`
}

// Less compares two Quest objects by ID.
func (d *Quest) Less(o *Quest) bool {
	return d.ID < o.ID
}

// QuestSlice is a slice of display Quests. It is always sorted.
type QuestSlice []*Quest

func (s QuestSlice) Len() int           { return len(s) }
func (s QuestSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s QuestSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get retrieves a Quest from this slice by ID.
func (s QuestSlice) Get(qid string) *Quest {
	idx := sort.Search(len(s), func(i int) bool {
		return s[i].ID >= qid
	})
	if idx < len(s) && s[idx].ID == qid {
		return s[idx]
	}
	return nil
}

// Merge inserts the quest `q` into this slice, preserving sorted order.
func (s *QuestSlice) Merge(q *Quest) *Quest {
	if q == nil {
		return nil
	}
	cur := s.Get(q.ID)
	if cur == nil {
		*s = append(*s, q)
		if len(*s) > 1 && q.Less((*s)[len(*s)-2]) {
			sort.Sort(*s)
		}
		return q
	}
	return nil
}
