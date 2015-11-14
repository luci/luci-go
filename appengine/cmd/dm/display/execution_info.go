// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
)

// ExecutionInfo is the json display object for an execution. It excludes the
// secret ExecutionKey field intentionally.
type ExecutionInfo struct {
	ExecutionID uint32 `endpoints:"desc=The Execution ID"`

	DistributorToken string `endpoints:"desc=The distributor Token that this execution has."`
}

// Less compares two ExecutionInfo's by their ExecutionID.
func (e *ExecutionInfo) Less(o *ExecutionInfo) bool {
	return e.ExecutionID < o.ExecutionID
}

// ExecutionInfoSlice is a slice of ExecutionInfos.
type ExecutionInfoSlice []*ExecutionInfo

func (s ExecutionInfoSlice) Len() int           { return len(s) }
func (s ExecutionInfoSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s ExecutionInfoSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get retrieves an ExecutionInfo from this slice by ExecutionID.
func (s ExecutionInfoSlice) Get(eid uint32) *ExecutionInfo {
	idx := sort.Search(len(s), func(i int) bool {
		return s[i].ExecutionID >= eid
	})
	if idx < len(s) && s[idx].ExecutionID == eid {
		return s[idx]
	}
	return nil
}

// Merge adds the provided execution into the slice (in sorted order). It
// returns the merged Execution if it was inserted. Otherwise it returns nil.
func (s *ExecutionInfoSlice) Merge(e *ExecutionInfo) *ExecutionInfo {
	if e == nil {
		return nil
	}
	cur := s.Get(e.ExecutionID)
	if cur == nil {
		*s = append(*s, e)
		if len(*s) > 1 && e.Less((*s)[len(*s)-2]) {
			sort.Sort(*s)
		}
		return e
	}
	return nil
}
