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
	"testing"

	"golang.org/x/net/context"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetadataStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Works", t, func() {
		s := MetadataStore{}

		// Empty at the start.
		metas, err := s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldBeNil)

		// Start creating metadata for 'a', but don't actually touch it.
		meta, err := s.UpdateMetadata(ctx, "a/", func(m *api.PrefixMetadata) error {
			return nil
		})
		So(err, ShouldBeNil)
		So(meta, ShouldBeNil) // it is missing

		// Still missing.
		metas, err = s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldBeNil)

		// Create metadata for 'a' for real this time.
		meta, err = s.UpdateMetadata(ctx, "a/", func(m *api.PrefixMetadata) error {
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

		// Fetching 'a' returns only 'a'.
		metas, err = s.GetMetadata(ctx, "a")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a})

		// Prefix matches respects '/'.
		metas, err = s.GetMetadata(ctx, "ab")
		So(err, ShouldBeNil)
		So(metas, ShouldBeNil)

		// Still only 'a'.
		metas, err = s.GetMetadata(ctx, "a/b")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a})

		// And now we also see 'a/b/c'.
		metas, err = s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a, expected_abc})

		// And that's all we can ever see, even if we do deeper.
		metas, err = s.GetMetadata(ctx, "a/b/c/d/e/f")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{expected_a, expected_abc})
	})

	Convey("Root metadata", t, func() {
		s := MetadataStore{}

		// Create the metadata for the root.
		rootMeta, err := s.UpdateMetadata(ctx, "", func(m *api.PrefixMetadata) error {
			m.UpdateUser = "user:root@example.com"
			return nil
		})
		So(err, ShouldBeNil)

		So(rootMeta, ShouldResemble, &api.PrefixMetadata{
			Fingerprint: "a7QYP7C3AXksn_pfotXl2OwBevc",
			UpdateUser:  "user:root@example.com",
		})

		// Fetchable now.
		metas, err := s.GetMetadata(ctx, "")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{rootMeta})

		// "/" is also accepted.
		metas, err = s.GetMetadata(ctx, "/")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{rootMeta})

		// Make sure UpdateMetadata see the root metadata too.
		_, err = s.UpdateMetadata(ctx, "", func(m *api.PrefixMetadata) error {
			So(m, ShouldResemble, rootMeta)
			return nil
		})
		So(err, ShouldBeNil)

		// Create metadata for some prefix.
		abMeta, err := s.UpdateMetadata(ctx, "a/b", func(m *api.PrefixMetadata) error {
			m.UpdateUser = "user:ab@example.com"
			return nil
		})
		So(err, ShouldBeNil)

		// Fetching meta for prefixes picks up root metadata too.
		metas, err = s.GetMetadata(ctx, "a")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{rootMeta})
		metas, err = s.GetMetadata(ctx, "a/b/c")
		So(err, ShouldBeNil)
		So(metas, ShouldResemble, []*api.PrefixMetadata{rootMeta, abMeta})
	})
}
