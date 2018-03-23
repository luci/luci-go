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

package repo

import (
	"sort"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/context"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/common"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

// metadataStore implements metadata.Storage in memory, for tests.
type metadataStore struct {
	l     sync.Mutex
	metas map[string]*api.PrefixMetadata // prefix with trailing '/' => metadata
}

// Populate is a shortcut for UpdateMetadata to populate the storage in tests.
func (s *metadataStore) Populate(prefix string, m *api.PrefixMetadata) *api.PrefixMetadata {
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
func (s *metadataStore) Purge(prefix string) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		panic(err)
	}
	prefix += "/"

	s.l.Lock()
	defer s.l.Unlock()

	delete(s.metas, prefix)
}

func (s *metadataStore) GetMetadata(c context.Context, prefix string) ([]*api.PrefixMetadata, error) {
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
			metas = append(metas, deepCopyMetadata(meta))
		}
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Prefix < metas[j].Prefix
	})
	return metas, nil
}

func (s *metadataStore) UpdateMetadata(c context.Context, prefix string, cb func(m *api.PrefixMetadata) error) (*api.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, err
	}
	prefix += "/"

	s.l.Lock()
	defer s.l.Unlock()

	meta := s.metas[prefix]
	if meta == nil {
		meta = &api.PrefixMetadata{
			Prefix: strings.TrimSuffix(prefix, "/"),
		}
	} else {
		meta = deepCopyMetadata(meta) // don't let the callback modify the internal data
	}

	if err := cb(meta); err != nil {
		return nil, err
	}
	meta.Prefix = strings.TrimSuffix(prefix, "/")
	meta.Fingerprint = metadata.CalculateFingerprint(meta)

	if s.metas == nil {
		s.metas = make(map[string]*api.PrefixMetadata, 1)
	}
	s.metas[prefix] = deepCopyMetadata(meta)

	return meta, nil
}

func deepCopyMetadata(m *api.PrefixMetadata) *api.PrefixMetadata {
	// So golang has no built-in deep copy method, and doing it field by field
	// manually is prone to forgetting some fields. Since it is only for tests,
	// do the copy in a really hacky way.
	blob, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	res := &api.PrefixMetadata{}
	if err := proto.Unmarshal(blob, res); err != nil {
		panic(err)
	}
	return res
}

func TestFakeMetadataStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Works", t, func() {
		s := metadataStore{}

		// Empty at the start.
		metas, err := s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldBeNil)

		// Create metadata for 'a'.
		meta, err := s.UpdateMetadata(ctx, "a/", func(m *api.PrefixMetadata) error {
			So(m.Prefix, ShouldEqual, "a")
			So(m.Fingerprint, ShouldEqual, "")
			m.UpdateUser = "user:a@example.com"
			return nil
		})
		So(err, ShouldBeNil)

		expected_a := &api.PrefixMetadata{
			Prefix:      "a",
			Fingerprint: "ccAI44xVAoO3SUzK2x6b0wZMD00",
			UpdateUser:  "user:a@example.com",
		}
		So(meta, ShouldResemble, expected_a)

		// Again, sees the updated metadata now.
		meta, err = s.UpdateMetadata(ctx, "a/", func(m *api.PrefixMetadata) error {
			So(m, ShouldResemble, expected_a)
			return nil
		})
		So(err, ShouldBeNil)
		So(meta, ShouldResemble, expected_a)

		// Create metadata for 'a/b/c'.
		meta, err = s.UpdateMetadata(ctx, "a/b/c", func(m *api.PrefixMetadata) error {
			m.UpdateUser = "user:abc@example.com"
			return nil
		})
		So(err, ShouldBeNil)

		expected_abc := &api.PrefixMetadata{
			Prefix:      "a/b/c",
			Fingerprint: "HZozZp-6ZMi8lZp11-w54xJBjhA",
			UpdateUser:  "user:abc@example.com",
		}
		So(meta, ShouldResemble, expected_abc)

		// Create metadata for 'a/b/d' (sibling), to make sure it will not appear
		// in responses below.
		_, err = s.UpdateMetadata(ctx, "a/b/d", func(m *api.PrefixMetadata) error {
			m.UpdateUser = "user:abd@example.com"
			return nil
		})
		So(err, ShouldBeNil)

		// Fetching root returns only root.
		metas, err = s.GetMetadata(ctx, "a")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a})

		// Prefix matches respects '/'.
		metas, err = s.GetMetadata(ctx, "ab")
		So(err, ShouldBeNil)
		So(metas, ShouldBeNil)

		// Still only root.
		metas, err = s.GetMetadata(ctx, "a/b")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a})

		// And now we also see a/b/c.
		metas, err = s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a, expected_abc})

		// And that's all we can ever see, even if we do deeper.
		metas, err = s.GetMetadata(ctx, "a/b/c/d/e/f")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a, expected_abc})
	})
}
