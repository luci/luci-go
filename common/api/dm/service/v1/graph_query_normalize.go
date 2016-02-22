// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"fmt"
)

// Normalize returns an error iff this GraphQuery is not valid.
func (g *GraphQuery) Normalize() error {
	query := g.Query
	if query == nil {
		return fmt.Errorf("invalid GraphQuery: no specified query")
	}
	return query.(interface {
		Normalize() error
	}).Normalize()
}

func (al *GraphQuery_AttemptList_) Normalize() error {
	return al.AttemptList.Normalize()
}

func (al *GraphQuery_AttemptRange_) Normalize() error {
	return al.AttemptRange.Normalize()
}

func (s *GraphQuery_Search_) Normalize() error {
	return s.Search.Normalize()
}

func (al *GraphQuery_AttemptList) Normalize() error {
	al.Attempt.Normalize()
	return nil
}

func (al *GraphQuery_AttemptRange) Normalize() error {
	if al.Quest == "" {
		return fmt.Errorf("must specify quest")
	}
	if al.Low == 0 {
		return fmt.Errorf("must specify low")
	}
	if al.High <= al.Low {
		return fmt.Errorf("high must be > low")
	}
	return nil
}

func (s *GraphQuery_Search) Normalize() error {
	// for now, start and end MUST be timestamp values.
	switch s.Start.Value.(type) {
	case *PropertyValue_Time:
	default:
		return fmt.Errorf("invalid Start type: %T", s.Start.Value)
	}
	switch s.End.Value.(type) {
	case *PropertyValue_Time:
	default:
		return fmt.Errorf("invalid End type: %T", s.End.Value)
	}
	return nil
}
