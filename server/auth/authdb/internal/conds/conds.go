// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package conds contains supporting code for conditional bindings.
//
// Used internally by authdb.Snapshot.
package conds

import (
	"bytes"
	"context"
	"encoding/binary"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// Condition is a predicate on attributes map.
type Condition struct {
	conds []elementary
	index int
}

// elementary evaluates a condition matching a single realms.Condition message.
type elementary func(ctx context.Context, attrs realms.Attrs) bool

// Eval evaluates the condition over given attributes.
func (c *Condition) Eval(ctx context.Context, attrs realms.Attrs) bool {
	for _, cond := range c.conds {
		if !cond(ctx, attrs) {
			return false
		}
	}
	return true
}

// Index can be used to sort conditions based on the order of their creation.
//
// Indexes are comparable only if conditions came from the same Builder.
func (c *Condition) Index() int {
	return c.index
}

// An elementary condition that always evaluates to `false`.
var alwaysFalse elementary = func(context.Context, realms.Attrs) bool {
	return false
}

// Builder is a caching factory of Conditions.
type Builder struct {
	vocab    []*protocol.Condition // available "vocabulary" of elementary conditions
	elems    []elementary          // lazily constructed elementary conditions
	cache    map[string]*Condition // cached composite (ANDed) conditions
	interned map[string]string     // for interning strings to save RAM
}

// NewBuilder returns a new builder that uses the given "vocabulary" of
// elementary conditions when building composite (ANDed) conditions.
//
// This is usually `conditions` field from the Realms proto message. These
// elementary conditions are referenced by their index in Condition(...) method.
func NewBuilder(vocab []*protocol.Condition) *Builder {
	return &Builder{
		vocab:    vocab,
		elems:    make([]elementary, len(vocab)),
		cache:    make(map[string]*Condition),
		interned: make(map[string]string),
	}
}

// Condition either constructs a new condition or returns an existing one.
//
// `indexes` are indexes of elementary conditions in the array passed to
// NewBuilder. The returned condition is an AND of all these elementary
// conditions. `indexes` are assumed to be ordered already.
//
// Returns nil if the condition would always evaluate to true (in particular if
// `indexes` is empty).
//
// Returns an error if some index is out of bounds. This should not happen in
// a valid AuthDB.
func (b *Builder) Condition(indexes []uint32) (*Condition, error) {
	if len(indexes) == 0 {
		return nil, nil
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(indexes)*4))
	if err := binary.Write(buf, binary.LittleEndian, indexes); err != nil {
		panic(err)
	}
	cacheKey := buf.String()

	if cond, exists := b.cache[cacheKey]; exists {
		return cond, nil
	}

	cond, err := b.build(indexes)
	if err != nil {
		return nil, err
	}
	cond.index = len(b.cache)
	b.cache[cacheKey] = cond
	return cond, nil
}

// A factory of Condition that builds it out of elementary conditions.
func (b *Builder) build(indexes []uint32) (*Condition, error) {
	elems := make([]elementary, len(indexes))
	for i, elemIdx := range indexes {
		var err error
		if elems[i], err = b.elementary(elemIdx); err != nil {
			return nil, err
		}
	}
	return &Condition{conds: elems}, nil
}

// A caching factory of elementary conditions based on their index in `vocab`.
func (b *Builder) elementary(index uint32) (elementary, error) {
	idx := int(index)
	if idx >= len(b.elems) {
		return nil, errors.Fmt("condition index is out of bounds: %d >= %d", idx, len(b.elems))
	}

	elem := b.elems[idx]
	if elem != nil {
		return elem, nil
	}

	switch typ := b.vocab[idx].Op.(type) {
	case *protocol.Condition_Restrict:
		elem = b.attributeRestriction(typ.Restrict)
	default:
		elem = b.unrecognized(b.vocab[idx])
	}

	b.elems[idx] = elem
	return elem, nil
}

// internStr returns an interned copy of `s`.
func (b *Builder) internStr(s string) string {
	if existing, ok := b.interned[s]; ok {
		return existing
	}
	b.interned[s] = s
	return s
}

// internSet returns a string set with interned copies of strings.
func (b *Builder) internSet(s []string) map[string]struct{} {
	out := make(map[string]struct{}, len(s))
	for _, val := range s {
		out[b.internStr(val)] = struct{}{}
	}
	return out
}

/// Factories of concrete types of elementary conditions.

func (b *Builder) attributeRestriction(cond *protocol.Condition_AttributeRestriction) elementary {
	if len(cond.Values) == 0 {
		return alwaysFalse
	}

	attr := b.internStr(cond.Attribute)
	vals := b.internSet(cond.Values)

	return func(ctx context.Context, attrs realms.Attrs) bool {
		if val, known := attrs[attr]; known {
			_, match := vals[val]
			return match
		}
		return false
	}
}

func (b *Builder) unrecognized(cond *protocol.Condition) elementary {
	return func(ctx context.Context, attrs realms.Attrs) bool {
		logging.Warningf(ctx, "Unrecognized or empty condition in a conditional binding, evaluating it to false: %q", cond)
		return false
	}
}
