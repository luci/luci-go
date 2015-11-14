// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// ExecutionsForAttempt is the json display struct for DM Executions.
type ExecutionsForAttempt struct {
	Attempt types.AttemptID

	Executions ExecutionInfoSlice
}

// Less compares two ExecutionsForAttempt, by their AttemptID.
func (d *ExecutionsForAttempt) Less(o *ExecutionsForAttempt) bool {
	return d.Attempt.Less(&o.Attempt)
}

// ExecutionsForAttemptSlice is a slice of ExecutionsForAttempts! It is always
// in sorted order.
type ExecutionsForAttemptSlice []*ExecutionsForAttempt

func (s ExecutionsForAttemptSlice) Len() int           { return len(s) }
func (s ExecutionsForAttemptSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s ExecutionsForAttemptSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get retrieves a specific ExecutionsForAttempt from the slice by Attempt and ExecutionID.
// If no matching ExecutionsForAttempt is found, this returns nil.
func (s ExecutionsForAttemptSlice) Get(aid *types.AttemptID) *ExecutionsForAttempt {
	idx := sort.Search(len(s), func(i int) bool {
		return !s[i].Attempt.Less(aid)
	})
	if idx < len(s) && s[idx].Attempt == *aid {
		return s[idx]
	}
	return nil
}

// Merge adds the provided ExecutionsForAttempt into the slice (in sorted order). It
// returns the merged ExecutionsForAttempt if it was inserted. Otherwise it returns nil.
func (s *ExecutionsForAttemptSlice) Merge(e *ExecutionsForAttempt) *ExecutionsForAttempt {
	if e == nil {
		return nil
	}
	cur := s.Get(&e.Attempt)
	if cur == nil {
		// add
		*s = append(*s, e)
		if len(*s) > 1 && e.Less((*s)[len(*s)-2]) {
			sort.Sort(*s)
		}
		return e
	}

	// merge
	ret := *e
	ret.Executions = nil
	for _, ei := range e.Executions {
		ret.Executions.Merge(cur.Executions.Merge(ei))
	}
	if len(ret.Executions) == 0 {
		return nil
	}
	return &ret
}
