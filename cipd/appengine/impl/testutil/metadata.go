// Copyright 2018 The LUCI Authors.
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

package testutil

import (
	"container/list"
	"sort"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/common"
)

// MetadataStore implements metadata.Storage using memory, for tests.
//
// Not terribly efficient, shouldn't be used with a large number of entries.
type MetadataStore struct {
	l      sync.Mutex
	metas  map[string]*api.PrefixMetadata // e.g. "/a/b/c/" => metadata
	sorted []string                       // sorted list of keys in 'metas'
}

// Populate adds a metadata entry to the storage.
//
// If populates Prefix and Fingerprint. Returns the added item. Panics if the
// prefix is bad or the given metadata is empty.
func (s *MetadataStore) Populate(prefix string, m *api.PrefixMetadata) *api.PrefixMetadata {
	meta, err := s.UpdateMetadata(context.Background(), prefix, func(e *api.PrefixMetadata) error {
		*e = *m
		return nil
	})
	if err != nil {
		panic(err)
	}
	if meta == nil {
		panic("Populate should be used only with non-empty metadata")
	}
	return meta
}

// Purge removes metadata entry for some prefix.
//
// Panics if the prefix is bad. Purging missing metadata is noop.
func (s *MetadataStore) Purge(prefix string) {
	prefix, err := normPrefix(prefix)
	if err != nil {
		panic(err)
	}

	s.l.Lock()
	defer s.l.Unlock()

	if _, ok := s.metas[prefix]; ok {
		delete(s.metas, prefix)
		s.rebuildSorted()
	}
}

// GetMetadata fetches metadata associated with the given prefix and all
// parent prefixes.
func (s *MetadataStore) GetMetadata(c context.Context, prefix string) ([]*api.PrefixMetadata, error) {
	prefix, err := normPrefix(prefix)
	if err != nil {
		return nil, err
	}

	s.l.Lock()
	defer s.l.Unlock()

	var metas []*api.PrefixMetadata
	for _, p := range s.sorted {
		if strings.HasPrefix(prefix, p) {
			metas = append(metas, cloneMetadata(s.metas[p]))
		}
	}
	return metas, nil
}

// VisitMetadata performs breadth-first enumeration of the metadata graph.
func (s *MetadataStore) VisitMetadata(c context.Context, prefix string, cb metadata.Visiter) error {
	prefix, err := normPrefix(prefix)
	if err != nil {
		return err
	}

	// Start with the requested prefix if it exists, otherwise start with all
	// its direct descendants.
	var next stringDeque
	if s.exists(prefix) {
		next.PushBack(prefix)
	} else {
		for _, p := range s.directDescendants(prefix) {
			next.PushBack(p)
		}
	}

	// Note that we don't hold the lock here, to avoid deadlocking if 'cb' calls
	// other MetadataStore methods.
	for next.Len() > 0 {
		pfx := next.PopFront()

		// Convert the prefix back to the form expected by the public API.
		clean := strings.TrimSuffix(strings.TrimPrefix(pfx, "/"), "/")

		md, err := s.GetMetadata(c, clean)
		if err != nil {
			return err
		}

		switch cont, err := cb(clean, md); {
		case err != nil:
			return err
		case cont:
			// Asked to explore this subtree, add the children to the queue of nodes
			// to visit.
			for _, p := range s.directDescendants(pfx) {
				next.PushBack(p)
			}
		}
	}
	return nil
}

// UpdateMetadata transactionally updates or creates metadata of some
// prefix.
func (s *MetadataStore) UpdateMetadata(c context.Context, prefix string, cb func(m *api.PrefixMetadata) error) (*api.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, err
	}

	s.l.Lock()
	defer s.l.Unlock()

	key, _ := normPrefix(prefix)
	before, existed := s.metas[key] // the metadata before the callback
	if !existed {
		before = &api.PrefixMetadata{Prefix: prefix}
	}

	// Don't let the callback modify or retain the internal data.
	meta := cloneMetadata(before)
	if err := cb(meta); err != nil {
		return nil, err
	}

	// Don't let the callback mess with the prefix or the fingerprint.
	meta.Prefix = before.Prefix
	meta.Fingerprint = before.Fingerprint

	// No changes at all? Return nil if the metadata didn't exist and wasn't
	// created by the callback. Otherwise return the existing metadata.
	if proto.Equal(before, meta) {
		if !existed {
			return nil, nil
		}
		return meta, nil
	}

	// Calculate the new fingerprint and put the metadata into the storage.
	meta.Fingerprint = metadata.CalculateFingerprint(*meta)
	if s.metas == nil {
		s.metas = make(map[string]*api.PrefixMetadata, 1)
	}
	s.metas[key] = cloneMetadata(meta)

	// Rebuild 'sorted'. Note that this can be optimized by using sort.Search and
	// insertion into the existing array. But we don't care, len(metas) should be
	// tiny.
	if !existed {
		s.rebuildSorted()
	}

	return meta, nil
}

// rebuildSorted recalculates s.sorted based on s.metas.
//
// Should be called under the lock.
func (s *MetadataStore) rebuildSorted() {
	s.sorted = s.sorted[:0]
	for k := range s.metas {
		s.sorted = append(s.sorted, k)
	}
	sort.Strings(s.sorted)
}

// exists returns true if metadata for prefix 'p' is registered.
//
// 'p' assumed to be already normalized via normPrefix.
func (s *MetadataStore) exists(p string) bool {
	s.l.Lock()
	defer s.l.Unlock()
	_, ok := s.metas[p]
	return ok
}

// directDescendants returns registered prefixes that are directly under 'p'.
//
// "Directly under" here means that if A is "directly under" B, then B is a
// prefix of A, and there's no such C that A is a prefix of C and C is a prefix
// of B.
//
// Both 'p' and returned prefixes assumed to be in the normalized form (see
// normPrefix). The returned list is sorted already.
func (s *MetadataStore) directDescendants(p string) []string {
	s.l.Lock()
	defer s.l.Unlock()

	// Note: this can be optimized using a smarter data structures and algorithms,
	// but we don't care since MetadataStore is for unit tests only.
	var out []string
	for _, pfx := range s.sorted {
		if p == pfx || !strings.HasPrefix(pfx, p) {
			continue
		}

		// Maybe we've discovered a smaller prefix already. If so, 'pfx' is not a
		// direct descendant and we should skip it.
		keep := true
		for _, discovered := range out {
			if strings.HasPrefix(pfx, discovered) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, pfx)
		}
	}
	return out
}

// normPrefix takes "a/b/c" and returns "/a/b/c/".
//
// For "" returns "/".
func normPrefix(p string) (string, error) {
	p, err := common.ValidatePackagePrefix(p)
	if err != nil {
		return "", err
	}
	if p == "" {
		return "/", nil
	}
	return "/" + p + "/", nil
}

// cloneMetadata makes a deep copy of 'm'.
func cloneMetadata(m *api.PrefixMetadata) *api.PrefixMetadata {
	return proto.Clone(m).(*api.PrefixMetadata)
}

////////////////////////////////////////////////////////////////////////////////

// stringDeque is a double-ended queue of strings.
type stringDeque struct {
	l list.List
}

func (d *stringDeque) Len() int {
	return d.l.Len()
}

func (d *stringDeque) PushBack(s string) {
	d.l.PushBack(s)
}

func (d *stringDeque) PopFront() string {
	e := d.l.Front()
	if e == nil {
		panic("popping from an empty stringDeque")
	}
	d.l.Remove(e)
	return e.Value.(string)
}
