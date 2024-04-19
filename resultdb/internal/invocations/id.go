// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
)

// ID can convert an invocation id to various formats.
type ID string

// ToSpanner implements span.Value.
func (id ID) ToSpanner() any {
	return id.RowID()
}

// SpannerPtr implements span.Ptr.
func (id *ID) SpannerPtr(b *spanutil.Buffer) any {
	return &b.NullString
}

// FromSpanner implements span.Ptr.
func (id *ID) FromSpanner(b *spanutil.Buffer) error {
	*id = ""
	if b.NullString.Valid {
		*id = IDFromRowID(b.NullString.StringVal)
	}
	return nil
}

// MustParseName converts an invocation name to an ID.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseName(name string) ID {
	id, err := pbutil.ParseInvocationName(name)
	if err != nil {
		panic(err)
	}
	return ID(id)
}

// IDFromRowID converts a Spanner-level row ID to an ID.
func IDFromRowID(rowID string) ID {
	return ID(stripHashPrefix(rowID))
}

// Name returns an invocation name.
func (id ID) Name() string {
	return pbutil.InvocationName(string(id))
}

// RowID returns an invocation ID used in spanner rows.
// If id is empty, returns "".
func (id ID) RowID() string {
	if id == "" {
		return ""
	}
	return prefixWithHash(string(id))
}

// Key returns a invocation spanner key.
func (id ID) Key(suffix ...any) spanner.Key {
	ret := make(spanner.Key, 1+len(suffix))
	ret[0] = id.RowID()
	copy(ret[1:], suffix)
	return ret
}

// IDSet is an unordered set of invocation ids.
type IDSet map[ID]struct{}

// NewIDSet creates an IDSet from members.
func NewIDSet(ids ...ID) IDSet {
	ret := make(IDSet, len(ids))
	for _, id := range ids {
		ret.Add(id)
	}
	return ret
}

// Add adds id to the set.
func (s IDSet) Add(id ID) {
	s[id] = struct{}{}
}

// Union adds other ids.
func (s IDSet) Union(other IDSet) {
	for id := range other {
		s.Add(id)
	}
}

// Intersect returns only the ids present in this set and other.
func (s IDSet) Intersect(other IDSet) IDSet {
	var smaller, larger IDSet
	if len(other) < len(s) {
		smaller, larger = other, s
	} else {
		smaller, larger = s, other
	}

	ret := make(IDSet)
	for id := range smaller {
		if larger.Has(id) {
			ret.Add(id)
		}
	}
	return ret
}

// RemoveAll removes any ids present in other.
func (s IDSet) RemoveAll(other IDSet) {
	if len(s) > 0 {
		for id := range other {
			s.Remove(id)
		}
	}
}

// Remove removes id from the set if it was present.
func (s IDSet) Remove(id ID) {
	delete(s, id)
}

// Has returns true if id is in the set.
func (s IDSet) Has(id ID) bool {
	_, ok := s[id]
	return ok
}

// String implements fmt.Stringer.
func (s IDSet) String() string {
	strs := make([]string, 0, len(s))
	for id := range s {
		strs = append(strs, string(id))
	}
	sort.Strings(strs)
	return fmt.Sprintf("%q", strs)
}

// Keys returns a spanner.KeySet.
func (s IDSet) Keys(suffix ...any) spanner.KeySet {
	ret := spanner.KeySets()
	for id := range s {
		ret = spanner.KeySets(id.Key(suffix...), ret)
	}
	return ret
}

// ToSpanner implements span.Value.
func (s IDSet) ToSpanner() any {
	ret := make([]string, 0, len(s))
	for id := range s {
		ret = append(ret, id.RowID())
	}
	sort.Strings(ret)
	return ret
}

// SpannerPtr implements span.Ptr.
func (s *IDSet) SpannerPtr(b *spanutil.Buffer) any {
	return &b.StringSlice
}

// FromSpanner implements span.Ptr.
func (s *IDSet) FromSpanner(b *spanutil.Buffer) error {
	*s = make(IDSet, len(b.StringSlice))
	for _, rowID := range b.StringSlice {
		s.Add(IDFromRowID(rowID))
	}
	return nil
}

// ParseNames converts invocation names to IDSet.
func ParseNames(names []string) (IDSet, error) {
	ids := make(IDSet, len(names))
	for _, name := range names {
		id, err := pbutil.ParseInvocationName(name)
		if err != nil {
			return nil, err
		}
		ids.Add(ID(id))
	}
	return ids, nil
}

// MustParseNames converts invocation names to IDSet.
// Panics if a name is invalid. Useful for situations when names were already
// validated.
func MustParseNames(names []string) IDSet {
	ids, err := ParseNames(names)
	if err != nil {
		panic(err)
	}
	return ids
}

// Names returns a sorted slice of invocation names.
func (s IDSet) Names() []string {
	names := make([]string, 0, len(s))
	for id := range s {
		names = append(names, id.Name())
	}
	sort.Strings(names)
	return names
}

// SortByRowID returns IDs in the set sorted by row id.
func (s IDSet) SortByRowID() []ID {
	rowIDs := make([]string, 0, len(s))
	for id := range s {
		rowIDs = append(rowIDs, id.RowID())
	}
	sort.Strings(rowIDs)

	ret := make([]ID, len(rowIDs))
	for i, rowID := range rowIDs {
		ret[i] = ID(stripHashPrefix(rowID))
	}
	return ret
}

// Batch returns IDs in the set in batches of size batchSize.
func (s IDSet) Batch(batchSize int) []IDSet {
	if batchSize <= 0 {
		panic("batchSize must be positive")
	}
	// Keep rows with similar IDs together, this minimises the number
	// of Spanner splits the batch will hit. It also ensures this
	// method is deterministic.
	ids := s.SortByRowID()
	result := make([]IDSet, 0, (len(ids)+batchSize-1)/batchSize)
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		result = append(result, NewIDSet(ids[start:end]...))
	}
	return result
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
