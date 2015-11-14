// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
)

// Distributor is the json display object for a single distributor's
// configuration.
type Distributor struct {
	Name string
	URL  string
}

// Less allows distributors to be compared based on their Name.
func (d *Distributor) Less(o *Distributor) bool {
	return d.Name < o.Name
}

// DistributorSlice is an always-sorted list of display Distributors.
type DistributorSlice []*Distributor

func (s DistributorSlice) Len() int           { return len(s) }
func (s DistributorSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s DistributorSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Get returns a pointer to the display Distributor in this slice, identified by
// its name, or nil, if it's not present.
func (s DistributorSlice) Get(n string) *Distributor {
	idx := sort.Search(len(s), func(i int) bool {
		return s[i].Name >= n
	})
	if idx < len(s) && s[idx].Name == n {
		return s[idx]
	}
	return nil
}

// Merge efficiently inserts the distributor d into this DistributorSlice, if
// it's not present.
func (s *DistributorSlice) Merge(d *Distributor) *Distributor {
	if d == nil {
		return nil
	}
	cur := s.Get(d.Name)
	if cur == nil {
		*s = append(*s, d)
		if len(*s) > 1 && d.Less((*s)[len(*s)-2]) {
			sort.Sort(*s)
		}
		return d
	}
	return nil
}
