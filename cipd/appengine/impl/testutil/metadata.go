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
	"context"
	"sort"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/common"
)

// MetadataStore implements metadata.Storage using memory, for tests.
//
// Not terribly efficient, shouldn't be used with a large number of entries.
type MetadataStore struct {
	l     sync.Mutex
	metas map[string]*repopb.PrefixMetadata // e.g. "/a/b/c/" => metadata
}

// Populate adds a metadata entry to the storage.
//
// If populates Prefix and Fingerprint. Returns the added item. Panics if the
// prefix is bad or the given metadata is empty.
func (s *MetadataStore) Populate(prefix string, m *repopb.PrefixMetadata) *repopb.PrefixMetadata {
	meta, err := s.UpdateMetadata(context.Background(), prefix, func(_ context.Context, e *repopb.PrefixMetadata) error {
		proto.Reset(e)
		proto.Merge(e, m)
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
	delete(s.metas, prefix)
}

// GetMetadata fetches metadata associated with the given prefix and all
// parent prefixes.
func (s *MetadataStore) GetMetadata(ctx context.Context, prefix string) ([]*repopb.PrefixMetadata, error) {
	prefix, err := normPrefix(prefix)
	if err != nil {
		return nil, err
	}

	s.l.Lock()
	defer s.l.Unlock()

	var metas []*repopb.PrefixMetadata
	for p := range s.metas {
		if strings.HasPrefix(prefix, p) {
			metas = append(metas, cloneMetadata(s.metas[p]))
		}
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Prefix < metas[j].Prefix
	})
	return metas, nil
}

// VisitMetadata performs depth-first enumeration of the metadata graph.
func (s *MetadataStore) VisitMetadata(ctx context.Context, prefix string, cb metadata.Visitor) error {
	prefix, err := normPrefix(prefix)
	if err != nil {
		return err
	}
	return s.asGraph(prefix).traverse(func(n *node) (cont bool, err error) {
		// If this node represents a path element without actual metadata attached
		// to it, just recurse deeper until we find some metadata. The exception is
		// the root prefix itself, since per VisitMetadata contract, we must visit
		// it even if it has no metadata directly attached to it.
		if !n.hasMeta && n.path != prefix {
			return true, nil
		}

		// Convert the prefix back to the form expected by the public API.
		clean := strings.Trim(n.path, "/")

		// Grab full metadata for it (including inherited one).
		md, err := s.GetMetadata(ctx, clean)
		if err != nil {
			return false, err
		}

		// And ask the callback whether we should proceed.
		return cb(clean, md)
	})
}

// UpdateMetadata transactionally updates or creates metadata of some
// prefix.
func (s *MetadataStore) UpdateMetadata(ctx context.Context, prefix string, cb func(ctx context.Context, m *repopb.PrefixMetadata) error) (*repopb.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, err
	}

	s.l.Lock()
	defer s.l.Unlock()

	key, _ := normPrefix(prefix)
	before := s.metas[key] // the metadata before the callback
	if before == nil {
		before = &repopb.PrefixMetadata{Prefix: prefix}
	}

	// Don't let the callback modify or retain the internal data.
	meta := cloneMetadata(before)
	if err := cb(ctx, meta); err != nil {
		return nil, err
	}

	// Don't let the callback mess with the prefix or the fingerprint.
	meta.Prefix = before.Prefix
	meta.Fingerprint = before.Fingerprint

	// No changes at all? Return nil if the metadata didn't exist and wasn't
	// created by the callback. Otherwise return the existing metadata.
	if proto.Equal(before, meta) {
		if before.Fingerprint == "" {
			return nil, nil
		}
		return meta, nil
	}

	// Calculate the new fingerprint and put the metadata into the storage.
	metadata.CalculateFingerprint(meta)
	if s.metas == nil {
		s.metas = make(map[string]*repopb.PrefixMetadata, 1)
	}
	s.metas[key] = cloneMetadata(meta)
	return meta, nil
}

////////////////////////////////////////////////////////////////////////////////

type node struct {
	path     string           // full path from the metadata root, e.g. "/a/b/c/"
	children map[string]*node // keys are elementary path components
	hasMeta  bool             // true if this node has metadata attached to it
}

// child returns a child node, creating it if necessary.
func (n *node) child(name string) *node {
	if c, ok := n.children[name]; ok {
		return c
	}
	if n.children == nil {
		n.children = make(map[string]*node, 1)
	}
	c := &node{path: n.path + name + "/"}
	n.children[name] = c
	return c
}

// traverse does depth-first traversal of the node's subtree starting from self.
//
// Children are visited in lexicographical order.
func (n *node) traverse(cb func(*node) (cont bool, err error)) error {
	switch descend, err := cb(n); {
	case err != nil:
		return err
	case !descend:
		return nil
	}

	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := n.children[k].traverse(cb); err != nil {
			return err
		}
	}
	return nil
}

// asGraph builds a graph representation of metadata subtree at the given
// prefix.
//
// The returned root node represents 'prefix'.
func (s *MetadataStore) asGraph(prefix string) *node {
	s.l.Lock()
	defer s.l.Unlock()

	root := &node{path: prefix}
	for pfx := range s.metas {
		if !strings.HasPrefix(pfx, prefix) {
			continue
		}
		// Convert "/<prefix>/a/b/c/" to "a/b/c".
		rel := strings.TrimRight(strings.TrimPrefix(pfx, prefix), "/")
		cur := root
		if rel != "" {
			for _, elem := range strings.Split(rel, "/") {
				cur = cur.child(elem)
			}
		}
		cur.hasMeta = true
	}
	return root
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
func cloneMetadata(m *repopb.PrefixMetadata) *repopb.PrefixMetadata {
	return proto.Clone(m).(*repopb.PrefixMetadata)
}
