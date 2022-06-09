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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTagIndex(t *testing.T) {
	t.Parallel()

	Convey("TagIndex", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("updateTagIndex", func() {
			Convey("create", func() {
				Convey("nil", func() {
					So(updateTagIndex(ctx, "key:val", 0, nil), ShouldBeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					So(datastore.Get(ctx, shd), ShouldErrLike, "no such entity")
				})

				Convey("empty", func() {
					ents := []TagIndexEntry{}
					So(updateTagIndex(ctx, "key:val", 0, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					So(datastore.Get(ctx, shd), ShouldErrLike, "no such entity")
				})

				Convey("one", func() {
					ents := []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					}
					So(updateTagIndex(ctx, "key:val", 0, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID: "key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					})
				})

				Convey("many", func() {
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
					So(updateTagIndex(ctx, "key:val", 0, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
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
					})
				})

				Convey("excessive", func() {
					ents := make([]TagIndexEntry, MaxTagIndexEntries+1)
					for i := range ents {
						ents[i] = TagIndexEntry{
							BuildID:  int64(i),
							BucketID: "bucket",
						}
					}
					So(updateTagIndex(ctx, "key:val", 0, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: "key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID:         "key:val",
						Incomplete: true,
					})
				})
			})

			Convey("update", func() {
				So(datastore.Put(ctx, &TagIndex{
					ID: ":1:key:val",
					Entries: []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					},
				}), ShouldBeNil)

				Convey("nil", func() {
					So(updateTagIndex(ctx, "key:val", 1, nil), ShouldBeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					})
				})

				Convey("empty", func() {
					ents := []TagIndexEntry{}
					So(updateTagIndex(ctx, "key:val", 1, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID: ":1:key:val",
						Entries: []TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "bucket",
							},
						},
					})
				})

				Convey("one", func() {
					ents := []TagIndexEntry{
						{
							BuildID:  2,
							BucketID: "bucket",
						},
					}
					So(updateTagIndex(ctx, "key:val", 1, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
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
					})
				})

				Convey("many", func() {
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
					So(updateTagIndex(ctx, "key:val", 1, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
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
					})
				})

				Convey("excessive", func() {
					ents := make([]TagIndexEntry, MaxTagIndexEntries)
					for i := range ents {
						ents[i] = TagIndexEntry{
							BuildID:  int64(i + 10),
							BucketID: "bucket",
						}
					}
					So(updateTagIndex(ctx, "key:val", 1, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":1:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID:         ":1:key:val",
						Incomplete: true,
					})
				})
			})

			Convey("incomplete", func() {
				So(datastore.Put(ctx, &TagIndex{
					ID:         ":2:key:val",
					Incomplete: true,
				}), ShouldBeNil)

				Convey("nil", func() {
					So(updateTagIndex(ctx, "key:val", 2, nil), ShouldBeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					})
				})

				Convey("empty", func() {
					ents := []TagIndexEntry{}
					So(updateTagIndex(ctx, "key:val", 2, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					})
				})

				Convey("one", func() {
					ents := []TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "bucket",
						},
					}
					So(updateTagIndex(ctx, "key:val", 2, ents), ShouldBeNil)

					shd := &TagIndex{
						ID: ":2:key:val",
					}
					So(datastore.Get(ctx, shd), ShouldBeNil)
					So(shd, ShouldResemble, &TagIndex{
						ID:         ":2:key:val",
						Incomplete: true,
					})
				})
			})
		})

		Convey("searchTagIndex", func() {
			So(datastore.Put(ctx, &TagIndex{
				ID:      ":1:buildset:patch/gerrit/chromium-review.googlesource.com/123/1",
				Entries: []TagIndexEntry{{BuildID: 123, BucketID: "proj/bkt"}},
			}), ShouldBeNil)
			Convey("found", func() {
				entries, err := SearchTagIndex(ctx, "buildset", "patch/gerrit/chromium-review.googlesource.com/123/1")
				So(err, ShouldBeNil)
				So(entries, ShouldResembleProto, []*TagIndexEntry{
					{BuildID: 123, BucketID: "proj/bkt"},
				})
			})

			Convey("not found", func() {
				entries, err := SearchTagIndex(ctx, "buildset", "not exist")
				So(err, ShouldBeNil)
				So(entries, ShouldBeNil)
			})

			Convey("bad TagIndexEntry", func() {
				So(datastore.Put(ctx, &TagIndex{
					ID:      "key:val",
					Entries: []TagIndexEntry{{BuildID: 123, BucketID: "/"}},
				}), ShouldBeNil)
				entries, err := SearchTagIndex(ctx, "key", "val")
				So(err, ShouldBeNil)
				So(entries, ShouldBeNil)
			})
		})
	})
}
