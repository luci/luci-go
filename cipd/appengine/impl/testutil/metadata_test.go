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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestMetadataStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	populateMD := func(s *MetadataStore, prefixes []string) {
		for _, p := range prefixes {
			s.Populate(p, &api.PrefixMetadata{UpdateUser: "user:someone@example.com"})
		}
	}

	getMD := func(s *MetadataStore, pfx string) []string {
		md, err := s.GetMetadata(ctx, pfx)
		assert.Loosely(t, err, should.BeNil)
		out := make([]string, len(md))
		for i, m := range md {
			out[i] = m.Prefix
		}
		return out
	}

	visitAll := func(s *MetadataStore, pfx string) (visited []string) {
		err := s.VisitMetadata(ctx, pfx, func(p string, md []*api.PrefixMetadata) (bool, error) {
			visited = append(visited, p)
			return true, nil
		})
		assert.Loosely(t, err, should.BeNil)
		return
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		s := MetadataStore{}

		// Empty at the start.
		metas, err := s.GetMetadata(ctx, "a/b/c")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.BeNil)

		// Start creating metadata for 'a', but don't actually touch it.
		meta, err := s.UpdateMetadata(ctx, "a/", func(_ context.Context, m *api.PrefixMetadata) error {
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, meta, should.BeNil) // it is missing

		// Still missing.
		metas, err = s.GetMetadata(ctx, "a/b/c")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.BeNil)

		// Create metadata for 'a' for real this time.
		meta, err = s.UpdateMetadata(ctx, "a/", func(_ context.Context, m *api.PrefixMetadata) error {
			assert.Loosely(t, m.Prefix, should.Equal("a"))
			assert.Loosely(t, m.Fingerprint, should.BeEmpty)
			m.UpdateUser = "user:a@example.com"
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		expected_a := &api.PrefixMetadata{
			Prefix:      "a",
			Fingerprint: "ccAI44xVAoO3SUzK2x6b0wZMD00",
			UpdateUser:  "user:a@example.com",
		}
		assert.Loosely(t, meta, should.Match(expected_a))

		// Again, sees the updated metadata now.
		meta, err = s.UpdateMetadata(ctx, "a/", func(_ context.Context, m *api.PrefixMetadata) error {
			assert.Loosely(t, m, should.Match(expected_a))
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, meta, should.Match(expected_a))

		// Create metadata for 'a/b/c'.
		meta, err = s.UpdateMetadata(ctx, "a/b/c", func(_ context.Context, m *api.PrefixMetadata) error {
			m.UpdateUser = "user:abc@example.com"
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		expected_abc := &api.PrefixMetadata{
			Prefix:      "a/b/c",
			Fingerprint: "HZozZp-6ZMi8lZp11-w54xJBjhA",
			UpdateUser:  "user:abc@example.com",
		}
		assert.Loosely(t, meta, should.Match(expected_abc))

		// Create metadata for 'a/b/d' (sibling), to make sure it will not appear
		// in responses below.
		_, err = s.UpdateMetadata(ctx, "a/b/d", func(_ context.Context, m *api.PrefixMetadata) error {
			m.UpdateUser = "user:abd@example.com"
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Fetching 'a' returns only 'a'.
		metas, err = s.GetMetadata(ctx, "a")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{expected_a}))

		// Prefix matches respects '/'.
		metas, err = s.GetMetadata(ctx, "ab")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.BeNil)

		// Still only 'a'.
		metas, err = s.GetMetadata(ctx, "a/b")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{expected_a}))

		// And now we also see 'a/b/c'.
		metas, err = s.GetMetadata(ctx, "a/b/c")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{expected_a, expected_abc}))

		// And that's all we can ever see, even if we do deeper.
		metas, err = s.GetMetadata(ctx, "a/b/c/d/e/f")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{expected_a, expected_abc}))
	})

	ftt.Run("Root metadata", t, func(t *ftt.Test) {
		s := MetadataStore{}

		// Create the metadata for the root.
		rootMeta, err := s.UpdateMetadata(ctx, "", func(_ context.Context, m *api.PrefixMetadata) error {
			m.UpdateUser = "user:root@example.com"
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, rootMeta, should.Match(&api.PrefixMetadata{
			Fingerprint: "a7QYP7C3AXksn_pfotXl2OwBevc",
			UpdateUser:  "user:root@example.com",
		}))

		// Fetchable now.
		metas, err := s.GetMetadata(ctx, "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{rootMeta}))

		// "/" is also accepted.
		metas, err = s.GetMetadata(ctx, "/")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{rootMeta}))

		// Make sure UpdateMetadata see the root metadata too.
		_, err = s.UpdateMetadata(ctx, "", func(_ context.Context, m *api.PrefixMetadata) error {
			assert.Loosely(t, m, should.Match(rootMeta))
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Create metadata for some prefix.
		abMeta, err := s.UpdateMetadata(ctx, "a/b", func(_ context.Context, m *api.PrefixMetadata) error {
			m.UpdateUser = "user:ab@example.com"
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Fetching meta for prefixes picks up root metadata too.
		metas, err = s.GetMetadata(ctx, "a")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{rootMeta}))
		metas, err = s.GetMetadata(ctx, "a/b/c")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metas, should.Match([]*api.PrefixMetadata{rootMeta, abMeta}))
	})

	ftt.Run("GetMetadata filters by prefix correctly", t, func(t *ftt.Test) {
		s := MetadataStore{}
		populateMD(&s, []string{"", "a", "ab", "a/b", "b", "a/b/c"})

		assert.Loosely(t, getMD(&s, ""), should.Match([]string{""}))
		assert.Loosely(t, getMD(&s, "a"), should.Match([]string{"", "a"}))
		assert.Loosely(t, getMD(&s, "a/b"), should.Match([]string{"", "a", "a/b"}))
		assert.Loosely(t, getMD(&s, "a/b/c"), should.Match([]string{"", "a", "a/b", "a/b/c"}))
	})

	ftt.Run("Purge works", t, func(t *ftt.Test) {
		s := MetadataStore{}
		populateMD(&s, []string{"", "a", "a/b", "a/b/c"})

		s.Purge("a/b")
		assert.Loosely(t, getMD(&s, "a/b/c"), should.Match([]string{"", "a", "a/b/c"}))

		s.Purge("")
		assert.Loosely(t, getMD(&s, "a/b/c"), should.Match([]string{"a", "a/b/c"}))

		s.Purge("a/b/c")
		assert.Loosely(t, getMD(&s, "a/b/c"), should.Match([]string{"a"}))

		s.Purge("a")
		assert.Loosely(t, getMD(&s, "a/b/c"), should.Match([]string{}))
	})

	ftt.Run("VisitMetadata visits", t, func(t *ftt.Test) {
		s := MetadataStore{}
		populateMD(&s, []string{
			"", "a", "ab", "a/b", "a/b/c",
			"b", "b/c/d/e", "b/c/d/f",
		})

		assert.Loosely(t, visitAll(&s, ""), should.Match([]string{
			"", "a", "a/b", "a/b/c", "ab", "b", "b/c/d/e", "b/c/d/f",
		}))
		assert.Loosely(t, visitAll(&s, "a"), should.Match([]string{"a", "a/b", "a/b/c"}))
		assert.Loosely(t, visitAll(&s, "a/b"), should.Match([]string{"a/b", "a/b/c"}))
	})

	ftt.Run("VisitMetadata always visits pfx even if it has no metadata", t, func(t *ftt.Test) {
		s := MetadataStore{}
		populateMD(&s, []string{"a", "ab", "a/b", "a/b/c", "a/c"})

		assert.Loosely(t, visitAll(&s, ""), should.Match([]string{
			"", "a", "a/b", "a/b/c", "a/c", "ab",
		}))
		assert.Loosely(t, visitAll(&s, "a/b/c/d"), should.Match([]string{"a/b/c/d"}))
		assert.Loosely(t, visitAll(&s, "some"), should.Match([]string{"some"}))
	})

	ftt.Run("VisitMetadata respects callback return value", t, func(t *ftt.Test) {
		s := MetadataStore{}
		populateMD(&s, []string{
			"1", "1/a", "1/a/b", "1/a/b/c",
			"2", "2/a", "2/a/b", "2/a/b/c",
		})

		var visited []string
		s.VisitMetadata(ctx, "", func(p string, md []*api.PrefixMetadata) (bool, error) {
			visited = append(visited, p)
			return len(md) <= 2, nil // explore no deeper than 3 levels
		})
		assert.Loosely(t, visited, should.Match([]string{
			"",
			"1", "1/a", "1/a/b",
			"2", "2/a", "2/a/b",
		}))
	})
}
