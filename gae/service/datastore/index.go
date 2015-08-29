// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"bytes"
)

const MaxIndexColumns = 64

type IndexDirection bool

const (
	// ASCENDING is false so that it's the default (zero) value.
	ASCENDING  IndexDirection = false
	DESCENDING                = true
)

func (i IndexDirection) String() string {
	if i == ASCENDING {
		return "ASCENDING"
	}
	return "DESCENDING"
}

type IndexColumn struct {
	Property  string
	Direction IndexDirection
}

func (i IndexColumn) cmp(o IndexColumn) int {
	// sort ascending first
	if i.Direction == ASCENDING && o.Direction == DESCENDING {
		return -1
	} else if i.Direction == DESCENDING && o.Direction == ASCENDING {
		return 1
	}
	return cmpString(i.Property, o.Property)()
}

type IndexDefinition struct {
	Kind     string
	Ancestor bool
	SortBy   []IndexColumn
}

func (id *IndexDefinition) Equal(o *IndexDefinition) bool {
	if id.Kind != o.Kind || id.Ancestor != o.Ancestor || len(id.SortBy) != len(o.SortBy) {
		return false
	}
	for i, col := range id.SortBy {
		if col != o.SortBy[i] {
			return false
		}
	}
	return true
}

// NormalizeOrder returns the normalized SortBy value for this IndexDefinition.
// This is just appending __key__ if it's not explicitly the last field in this
// IndexDefinition.
func (id *IndexDefinition) Normalize() *IndexDefinition {
	if len(id.SortBy) > 0 && id.SortBy[len(id.SortBy)-1].Property == "__key__" {
		return id
	}
	ret := *id
	ret.SortBy = make([]IndexColumn, len(id.SortBy), len(id.SortBy)+1)
	copy(ret.SortBy, id.SortBy)
	ret.SortBy = append(ret.SortBy, IndexColumn{Property: "__key__"})
	return &ret
}

func (id *IndexDefinition) GetFullSortOrder() []IndexColumn {
	id = id.Normalize()
	if !id.Ancestor {
		return id.SortBy
	}
	ret := make([]IndexColumn, 0, len(id.SortBy)+1)
	ret = append(ret, IndexColumn{Property: "__ancestor__"})
	return append(ret, id.SortBy...)
}

func (id *IndexDefinition) PrepForIdxTable() *IndexDefinition {
	return id.Normalize().Flip()
}

func (id *IndexDefinition) Flip() *IndexDefinition {
	ret := *id
	ret.SortBy = make([]IndexColumn, 0, len(id.SortBy))
	for i := len(id.SortBy) - 1; i >= 0; i-- {
		ret.SortBy = append(ret.SortBy, id.SortBy[i])
	}
	return &ret
}

// Yeah who needs templates, right?
// <flames>This is fine.</flames>

func cmpBool(a, b bool) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a && !b { // >
			return 1
		}
		return -1
	}
}

func cmpInt(a, b int) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}
}

func cmpString(a, b string) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}
}

func (i *IndexDefinition) Less(o *IndexDefinition) bool {
	decide := func(v int) (ret, keepGoing bool) {
		if v > 0 {
			return false, false
		}
		if v < 0 {
			return true, false
		}
		return false, true
	}

	factors := []func() int{
		cmpBool(i.Builtin(), o.Builtin()),
		cmpString(i.Kind, o.Kind),
		cmpBool(i.Ancestor, o.Ancestor),
		cmpInt(len(i.SortBy), len(o.SortBy)),
	}
	for _, f := range factors {
		ret, keepGoing := decide(f())
		if !keepGoing {
			return ret
		}
	}
	for idx := range i.SortBy {
		ret, keepGoing := decide(i.SortBy[idx].cmp(o.SortBy[idx]))
		if !keepGoing {
			return ret
		}
	}
	return false
}

func (i *IndexDefinition) Builtin() bool {
	return !i.Ancestor && len(i.SortBy) <= 1
}

func (i *IndexDefinition) Compound() bool {
	if i.Kind == "" || len(i.SortBy) <= 1 {
		return false
	}
	for _, sb := range i.SortBy {
		if sb.Property == "" || sb.Property == "__ancestor__" {
			return false
		}
	}
	return true
}

func (i *IndexDefinition) String() string {
	ret := &bytes.Buffer{}
	if i.Builtin() {
		ret.WriteRune('B')
	} else {
		ret.WriteRune('C')
	}
	ret.WriteRune(':')
	ret.WriteString(i.Kind)
	if i.Ancestor {
		ret.WriteString("|A")
	}
	for _, sb := range i.SortBy {
		ret.WriteRune('/')
		if sb.Direction == DESCENDING {
			ret.WriteRune('-')
		}
		ret.WriteString(sb.Property)
	}
	return ret.String()
}
