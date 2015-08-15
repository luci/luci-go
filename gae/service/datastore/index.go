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
		if sb.Property == "" {
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

func IndexBuiltinQueryPrefix() []byte {
	return []byte{0}
}

func IndexComplexQueryPrefix() []byte {
	return []byte{1}
}
