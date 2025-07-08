// Copyright 2025 The LUCI Authors.
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

package rootinvocations

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
)

// ID represents a Root Invocation identifier.
// It implements spanutil.Value and spanutil.Ptr.
type ID string

// ToSpanner implements spanutil.Value.
func (id ID) ToSpanner() any {
	return id.RowID()
}

// SpannerPtr implements spanutil.Ptr.
func (id *ID) SpannerPtr(b *spanutil.Buffer) any {
	return &b.NullString
}

// FromSpanner implements spanutil.Ptr.
func (id *ID) FromSpanner(b *spanutil.Buffer) error {
	*id = ""
	if b.NullString.Valid {
		*id = IDFromRowID(b.NullString.StringVal)
	}
	return nil
}

// shardID returns a value in [0,shardCount) deterministically based on the ID value.
func (id ID) shardID(shardCount int) int64 {
	hash := sha256.Sum256([]byte(id))
	val := binary.BigEndian.Uint32(hash[:4])
	return int64(val % uint32(shardCount))
}

// IDFromRowID converts a Spanner-level row ID to an ID.
func IDFromRowID(rowID string) ID {
	return ID(spanutil.StripHashPrefix(rowID))
}

// Name returns a root invocation name.
func (id ID) Name() string {
	return pbutil.RootInvocationName(string(id))
}

// RowID returns a root invocation ID used in spanner rows.
// If id is empty, returns "".
func (id ID) RowID() string {
	if id == "" {
		return ""
	}
	return spanutil.PrefixWithHash(string(id))
}

// Key returns a root invocation spanner key.
func (id ID) Key(suffix ...any) spanner.Key {
	ret := make(spanner.Key, 1+len(suffix))
	ret[0] = id.RowID()
	copy(ret[1:], suffix)
	return ret
}

// IDSet is an unordered set of root invocation ids.
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

// ToSpanner implements spanutil.Value.
func (s IDSet) ToSpanner() any {
	ret := make([]string, 0, len(s))
	for id := range s {
		ret = append(ret, id.RowID())
	}
	sort.Strings(ret)
	return ret
}

// SpannerPtr implements spanutil.Ptr.
func (s *IDSet) SpannerPtr(b *spanutil.Buffer) any {
	return &b.StringSlice
}

// FromSpanner implements spanutil.Ptr.
func (s *IDSet) FromSpanner(b *spanutil.Buffer) error {
	*s = make(IDSet, len(b.StringSlice))
	for _, rowID := range b.StringSlice {
		s.Add(IDFromRowID(rowID))
	}
	return nil
}
