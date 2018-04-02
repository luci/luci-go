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
type MetadataStore struct {
	l     sync.Mutex
	metas map[string]*api.PrefixMetadata // prefix with trailing '/' => metadata
}

// Populate adds a metadata entry to the storage.
//
// If populates Prefix and Fingerprint. Returns the added item. Panics if the
// prefix is bad.
func (s *MetadataStore) Populate(prefix string, m *api.PrefixMetadata) *api.PrefixMetadata {
	meta, err := s.UpdateMetadata(context.Background(), prefix, func(e *api.PrefixMetadata) error {
		*e = *m
		return nil
	})
	if err != nil {
		panic(err)
	}
	return meta
}

// Purge removes metadata entry for some prefix.
//
// Panics if the prefix is bad. Purging missing metadata is noop.
func (s *MetadataStore) Purge(prefix string) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		panic(err)
	}
	prefix += "/"

	s.l.Lock()
	defer s.l.Unlock()

	delete(s.metas, prefix)
}

// GetMetadata fetches metadata associated with the given prefix and all
// parent prefixes.
func (s *MetadataStore) GetMetadata(c context.Context, prefix string) ([]*api.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, err
	}
	prefix += "/"

	s.l.Lock()
	defer s.l.Unlock()

	var metas []*api.PrefixMetadata
	for p, meta := range s.metas {
		if strings.HasPrefix(prefix, p) {
			metas = append(metas, cloneMetadata(meta))
		}
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Prefix < metas[j].Prefix
	})
	return metas, nil
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

	key := prefix + "/"
	meta := s.metas[key]
	if meta == nil {
		meta = &api.PrefixMetadata{Prefix: prefix}
	} else {
		// Don't let the callback to modify or retain the internal data.
		meta = cloneMetadata(meta)
	}

	if err := cb(meta); err != nil {
		return nil, err
	}
	meta.Prefix = prefix
	meta.Fingerprint = metadata.CalculateFingerprint(*meta)

	if s.metas == nil {
		s.metas = make(map[string]*api.PrefixMetadata, 1)
	}
	s.metas[key] = cloneMetadata(meta)

	return meta, nil
}

func cloneMetadata(m *api.PrefixMetadata) *api.PrefixMetadata {
	return proto.Clone(m).(*api.PrefixMetadata)
}
