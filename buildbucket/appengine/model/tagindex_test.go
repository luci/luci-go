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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestTagIndex(t *testing.T) {
	t.Parallel()

	ftt.Run("TagIndex", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("updateTagIndex", func(t *ftt.Test) {
			t.Run("create", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 0, nil), should.BeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.ErrLike("no such entity"))
				})

				t.Run("empty", func(t *ftt.Test) {
					ents := []TagIndexEntry{}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 0, ents), should.BeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.ErrLike("no such entity"))
				})

				t.Run("one", func(t *ftt.Test) {
					ents := []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 0, ents), should.BeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: "key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("many", func(t *ftt.Test) {
					ents := []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
						{
							BuildID:  2,
							BucketID: "bucket",
						},
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 0, ents), should.BeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: "key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
							{
								BuildID:  2,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("excessive", func(t *ftt.Test) {
					ents := make([]TagIndexEntry, MaxTagIndexEntries+1)
					for i := range ents {
						ents[i] = TagIndexEntry{
							BuildID:  int64(i),
							BucketID: "bucket",
						}
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 0, ents), should.BeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID:         "key:val",
						Incomplete: true,
					}))
				})
			})

			t.Run("update", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &TagIndex{
					ID: ":1:key:val",
					Entries: []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					},
				}), should.BeNil)

				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 1, nil), should.BeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("empty", func(t *ftt.Test) {
					ents := []TagIndexEntry{}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 1, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("one", func(t *ftt.Test) {
					ents := []TagIndexEntry{
						{
							BuildID:  2,
							BucketID: "bucket",
						},
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 1, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
							{
								BuildID:  2,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("many", func(t *ftt.Test) {
					ents := []TagIndexEntry{
						{
							BuildID:  2,
							BucketID: "bucket",
						},
						{
							BuildID:  3,
							BucketID: "bucket",
						},
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 1, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
							{
								BuildID:  2,
								BucketID: "bucket",
							},
							{
								BuildID:  3,
								BucketID: "bucket",
							},
						},
					}))
				})

				t.Run("excessive", func(t *ftt.Test) {
					ents := make([]TagIndexEntry, MaxTagIndexEntries)
					for i := range ents {
						ents[i] = TagIndexEntry{
							BuildID:  int64(i + 10),
							BucketID: "bucket",
						}
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 1, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID:         ":1:key:val",
						Incomplete: true,
					}))
				})
			})

			t.Run("incomplete", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &TagIndex{
					ID:         ":2:key:val",
					Incomplete: true,
				}), should.BeNil)

				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 2, nil), should.BeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					}))
				})

				t.Run("empty", func(t *ftt.Test) {
					ents := []TagIndexEntry{}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 2, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					}))
				})

				t.Run("one", func(t *ftt.Test) {
					ents := []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					}
					assert.Loosely(t, updateTagIndex(ctx, "key:val", 2, ents), should.BeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					assert.Loosely(t, datastore.Get(ctx, shd), should.BeNil)
					assert.Loosely(t, shd, should.Resemble(&TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					}))
				})
			})
		})

		t.Run("searchTagIndex", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &TagIndex{
				ID:      ":1:buildset:patch/gerrit/chromium-review.googlesource.com/123/1",
				Entries: []TagIndexEntry{{BuildID: 123, BucketID: "proj/bkt"}},
			}), should.BeNil)
			t.Run("found", func(t *ftt.Test) {
				entries, err := SearchTagIndex(ctx, "buildset", "patch/gerrit/chromium-review.googlesource.com/123/1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entries, should.Resemble([]*TagIndexEntry{
					{BuildID: 123, BucketID: "proj/bkt"},
				}))
			})

			t.Run("not found", func(t *ftt.Test) {
				entries, err := SearchTagIndex(ctx, "buildset", "not exist")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entries, should.BeNil)
			})

			t.Run("bad TagIndexEntry", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &TagIndex{
					ID:      "key:val",
					Entries: []TagIndexEntry{{BuildID: 123, BucketID: "/"}},
				}), should.BeNil)
				entries, err := SearchTagIndex(ctx, "key", "val")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entries, should.BeNil)
			})
		})
	})
}
