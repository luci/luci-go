// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"bytes"
	"fmt"
	"strings"
)

// IndexColumn represents a sort order for a single entity field.
type IndexColumn struct {
	Property   string
	Descending bool
}

// ParseIndexColumn takes a spec in the form of /\s*-?\s*.+\s*/, and
// returns an IndexColumn. Examples are:
//   `- Field `:  IndexColumn{Property: "Field", Descending: true}
//   `Something`: IndexColumn{Property: "Something", Descending: false}
//
// `+Field` is invalid. `` is invalid.
func ParseIndexColumn(spec string) (IndexColumn, error) {
	col := IndexColumn{}
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "-") {
		col.Descending = true
		col.Property = strings.TrimSpace(spec[1:])
	} else if strings.HasPrefix(spec, "+") {
		return col, fmt.Errorf("datastore: invalid order: %q", spec)
	} else {
		col.Property = strings.TrimSpace(spec)
	}
	if col.Property == "" {
		return col, fmt.Errorf("datastore: empty order: %q", spec)
	}
	return col, nil
}

func (i IndexColumn) cmp(o IndexColumn) int {
	// sort ascending first
	if !i.Descending && o.Descending {
		return -1
	} else if i.Descending && !o.Descending {
		return 1
	}
	return cmpString(i.Property, o.Property)()
}

// String returns a human-readable version of this IndexColumn which is
// compatible with ParseIndexColumn.
func (i IndexColumn) String() string {
	ret := ""
	if i.Descending {
		ret = "-"
	}
	return ret + i.Property
}

// GQL returns a correctly formatted Cloud Datastore GQL literal which
// is valid for the `ORDER BY` clause.
//
// The flavor of GQL that this emits is defined here:
//   https://cloud.google.com/datastore/docs/apis/gql/gql_reference
func (i IndexColumn) GQL() string {
	if i.Descending {
		return gqlQuoteName(i.Property) + " DESC"
	}
	return gqlQuoteName(i.Property)
}

// IndexDefinition holds the parsed definition of a datastore index definition.
type IndexDefinition struct {
	Kind     string
	Ancestor bool
	SortBy   []IndexColumn
}

// Equal returns true if the two IndexDefinitions are equivalent.
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

// Normalize returns an IndexDefinition which has a normalized SortBy field.
//
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

// GetFullSortOrder gets the full sort order for this IndexDefinition,
// including an extra "__ancestor__" column at the front if this index has
// Ancestor set to true.
func (id *IndexDefinition) GetFullSortOrder() []IndexColumn {
	id = id.Normalize()
	if !id.Ancestor {
		return id.SortBy
	}
	ret := make([]IndexColumn, 0, len(id.SortBy)+1)
	ret = append(ret, IndexColumn{Property: "__ancestor__"})
	return append(ret, id.SortBy...)
}

// PrepForIdxTable normalize and then flips the IndexDefinition.
func (id *IndexDefinition) PrepForIdxTable() *IndexDefinition {
	return id.Normalize().Flip()
}

// Flip returns an IndexDefinition with its SortBy field in reverse order.
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

// Less returns true iff id is ordered before o.
func (id *IndexDefinition) Less(o *IndexDefinition) bool {
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
		cmpBool(id.Builtin(), o.Builtin()),
		cmpString(id.Kind, o.Kind),
		cmpBool(id.Ancestor, o.Ancestor),
		cmpInt(len(id.SortBy), len(o.SortBy)),
	}
	for _, f := range factors {
		ret, keepGoing := decide(f())
		if !keepGoing {
			return ret
		}
	}
	for idx := range id.SortBy {
		ret, keepGoing := decide(id.SortBy[idx].cmp(o.SortBy[idx]))
		if !keepGoing {
			return ret
		}
	}
	return false
}

// Builtin returns true iff the IndexDefinition is one of the automatic built-in
// indexes.
func (id *IndexDefinition) Builtin() bool {
	return !id.Ancestor && len(id.SortBy) <= 1
}

// Compound returns true iff this IndexDefinition is a valid compound index
// definition.
//
// NOTE: !Builtin() does not imply Compound().
func (id *IndexDefinition) Compound() bool {
	if id.Kind == "" || len(id.SortBy) <= 1 {
		return false
	}
	for _, sb := range id.SortBy {
		if sb.Property == "" || sb.Property == "__ancestor__" {
			return false
		}
	}
	return true
}

func (id *IndexDefinition) String() string {
	ret := &bytes.Buffer{}
	wr := func(r rune) {
		_, err := ret.WriteRune(r)
		if err != nil {
			panic(err)
		}
	}

	ws := func(s string) {
		_, err := ret.WriteString(s)
		if err != nil {
			panic(err)
		}
	}

	if id.Builtin() {
		wr('B')
	} else {
		wr('C')
	}
	wr(':')
	ws(id.Kind)
	if id.Ancestor {
		ws("|A")
	}
	for _, sb := range id.SortBy {
		wr('/')
		if sb.Descending {
			wr('-')
		}
		ws(sb.Property)
	}
	return ret.String()
}
