// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"bytes"
)

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

type IndexDefinition struct {
	Kind     string
	Ancestor bool
	SortBy   []IndexColumn
}

func (i *IndexDefinition) Less(o *IndexDefinition) bool {
	// yes, this is inefficient.... however I'm disinclined to care, because the
	// actual comparison function is really ugly, and sorting IndexDefintions is
	// not performance critical. If you're here because you profiled this and
	// determined that it's a bottleneck, then feel free to rewrite :).
	//
	// At the time of writing, this function is only used during the tests of
	// impl/memory and this package.
	ibuf, obuf := &bytes.Buffer{}, &bytes.Buffer{}
	// we know these can't return an error because we're using bytes.Buffer
	_ = i.Write(ibuf)
	_ = o.Write(obuf)
	return bytes.Compare(ibuf.Bytes(), obuf.Bytes()) < 0
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
