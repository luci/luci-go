// Copyright 2019 The LUCI Authors.
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

package span

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/pbutil"
)

// InvocationID can convert an invocation id to various formats.
type InvocationID string

// ToSpanner implements span.Value.
func (id InvocationID) ToSpanner() interface{} {
	return id.RowID()
}

// SpannerPtr implements span.Ptr.
func (id *InvocationID) SpannerPtr(b *Buffer) interface{} {
	return &b.NullString
}

// FromSpanner implements span.Ptr.
func (id *InvocationID) FromSpanner(b *Buffer) error {
	*id = ""
	if b.NullString.Valid {
		*id = InvocationIDFromRowID(b.NullString.StringVal)
	}
	return nil
}

// MustParseInvocationName converts an invocation name to an InvocationID.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseInvocationName(name string) InvocationID {
	id, err := pbutil.ParseInvocationName(name)
	if err != nil {
		panic(err)
	}
	return InvocationID(id)
}

// InvocationIDFromRowID converts a Spanner-level row ID to an InvocationID.
func InvocationIDFromRowID(rowID string) InvocationID {
	return InvocationID(stripHashPrefix(rowID))
}

// Name returns an invocation name.
func (id InvocationID) Name() string {
	return pbutil.InvocationName(string(id))
}

// RowID returns an invocation ID used in spanner rows.
// If id is empty, returns "".
func (id InvocationID) RowID() string {
	if id == "" {
		return ""
	}
	return prefixWithHash(string(id))
}

// Key returns a invocation spanner key.
func (id InvocationID) Key(suffix ...interface{}) spanner.Key {
	ret := make(spanner.Key, 1+len(suffix))
	ret[0] = id.RowID()
	copy(ret[1:], suffix)
	return ret
}

// InvocationIDSet is an unordered set of invocation ids.
type InvocationIDSet map[InvocationID]struct{}

// NewInvocationIDSet creates an InvocationIDSet from members.
func NewInvocationIDSet(ids ...InvocationID) InvocationIDSet {
	ret := make(InvocationIDSet, len(ids))
	for _, id := range ids {
		ret.Add(id)
	}
	return ret
}

// Add adds id to the set.
func (s InvocationIDSet) Add(id InvocationID) {
	s[id] = struct{}{}
}

// Remove removes id from the set if it was present.
func (s InvocationIDSet) Remove(id InvocationID) {
	delete(s, id)
}

// Has returns true if id is in the set.
func (s InvocationIDSet) Has(id InvocationID) bool {
	_, ok := s[id]
	return ok
}

// String implements fmt.Stringer.
func (s InvocationIDSet) String() string {
	strs := make([]string, 0, len(s))
	for id := range s {
		strs = append(strs, string(id))
	}
	sort.Strings(strs)
	return fmt.Sprintf("%q", strs)
}

// Keys returns a spanner.KeySet.
func (s InvocationIDSet) Keys(suffix ...interface{}) spanner.KeySet {
	ret := spanner.KeySets()
	for id := range s {
		ret = spanner.KeySets(id.Key(suffix...), ret)
	}
	return ret
}

// ToSpanner implements span.Value.
func (s InvocationIDSet) ToSpanner() interface{} {
	ret := make([]string, 0, len(s))
	for id := range s {
		ret = append(ret, id.RowID())
	}
	sort.Strings(ret)
	return ret
}

// SpannerPtr implements span.Ptr.
func (s *InvocationIDSet) SpannerPtr(b *Buffer) interface{} {
	return &b.StringSlice
}

// FromSpanner implements span.Ptr.
func (s *InvocationIDSet) FromSpanner(b *Buffer) error {
	*s = make(InvocationIDSet, len(b.StringSlice))
	for _, rowID := range b.StringSlice {
		s.Add(InvocationIDFromRowID(rowID))
	}
	return nil
}

// MustParseInvocationNames converts invocation names to InvocationIDSet.
// Panics if a name is invalid. Useful for situations when names were already
// validated.
func MustParseInvocationNames(names []string) InvocationIDSet {
	ids := make(InvocationIDSet, len(names))
	for _, name := range names {
		ids.Add(MustParseInvocationName(name))
	}
	return ids
}

// Names returns a sorted slice of invocation names.
func (s InvocationIDSet) Names() []string {
	names := make([]string, 0, len(s))
	for id := range s {
		names = append(names, id.Name())
	}
	sort.Strings(names)
	return names
}

// hashPrefixBytes is the number of bytes of sha256 to prepend to a PK
// to achieve even distribution.
const hashPrefixBytes = 4

func prefixWithHash(s string) string {
	h := sha256.Sum256([]byte(s))
	prefix := hex.EncodeToString(h[:hashPrefixBytes])
	return fmt.Sprintf("%s:%s", prefix, s)
}

func stripHashPrefix(s string) string {
	expectedPrefixLen := hex.EncodedLen(hashPrefixBytes) + 1 // +1 for separator
	if len(s) < expectedPrefixLen {
		panic(fmt.Sprintf("%q is too short", s))
	}
	return s[expectedPrefixLen:]
}
