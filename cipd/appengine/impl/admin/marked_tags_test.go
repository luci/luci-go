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

package admin

import (
	"strings"
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVisitAndMarkTags(t *testing.T) {
	t.Parallel()

	Convey("With a bunch of instances", t, func() {
		ctx := gaetesting.TestingContext()

		makeInst := func(pkg, iid string) *model.Instance {
			i := &model.Instance{
				InstanceID: iid,
				Package:    model.PackageKey(ctx, pkg),
			}
			So(datastore.Put(ctx, i), ShouldBeNil)
			return i
		}

		tagKey := func(i *model.Instance, tag string) *datastore.Key {
			t, err := common.ParseInstanceTag(tag)
			So(err, ShouldBeNil)
			return datastore.KeyForObj(ctx, &model.Tag{
				ID:       model.TagID(t),
				Instance: datastore.KeyForObj(ctx, i),
			})
		}

		attachTag := func(i *model.Instance, tag string) *datastore.Key {
			t, err := common.ParseInstanceTag(tag)
			So(err, ShouldBeNil)
			So(model.AttachTags(ctx, i, []*api.Tag{t}), ShouldBeNil)
			return tagKey(i, tag)
		}

		// Create a bunch of instances to attach tags to.
		i1 := makeInst("a/b", strings.Repeat("a", 40))
		i2 := makeInst("a/b", strings.Repeat("b", 40))
		i3 := makeInst("c", strings.Repeat("a", 40))

		Convey("Happy path", func() {
			// Actually create tags.
			keys := []*datastore.Key{
				attachTag(i1, "k:1"),
				attachTag(i1, "k:2"),
				attachTag(i2, "k:1"),
				attachTag(i3, "k:2"),
			}

			// Mark all k:1 tags.
			err := visitAndMarkTags(ctx, 1, keys, func(t *model.Tag) string {
				if t.Tag == "k:1" {
					return "why not?"
				}
				return ""
			})
			So(err, ShouldBeNil)

			// Mark all tags (in a different job).
			err = visitAndMarkTags(ctx, 2, keys, func(t *model.Tag) string {
				return "all"
			})
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()

			// Correctly have only 1 tags (and only them) in job #1 output!
			var marked []markedTag
			So(datastore.GetAll(ctx, queryMarkedTags(1), &marked), ShouldBeNil)
			So(marked, ShouldResemble, []markedTag{
				{
					ID:  "AwS3ochdSStSG0z9F5GXpjXLamSw0gUUPVK1oMTWlbg",
					Job: 1,
					Key: keys[2],
					Tag: "k:1",
					Why: "why not?",
				},
				{
					ID:  "SimACx5B4X3Tuj0OfceIMElLFqApBGVSo4TZbvXYgT0",
					Job: 1,
					Key: keys[0],
					Tag: "k:1",
					Why: "why not?",
				},
			})

			// Job #2 has collected all tags. And IDs are different from job #1.
			marked = nil
			So(datastore.GetAll(ctx, queryMarkedTags(2), &marked), ShouldBeNil)
			So(marked, ShouldResemble, []markedTag{
				{
					ID:  "1ZOT2ST48S2gc2qrcKcS5C8g18roHwMhujwkluTSx8s",
					Job: 2,
					Key: keys[3],
					Tag: "k:2",
					Why: "all",
				},
				{
					ID:  "6LLWJBu6MSLEhGMtN6Y4lbhncII_DhlLT1J3CtbrZug",
					Job: 2,
					Key: keys[0],
					Tag: "k:1",
					Why: "all",
				},
				{
					ID:  "Klf5JuIRstnysbi_FrSJ7DQnQQDrbbfu-rnsezppZXU",
					Job: 2,
					Key: keys[2],
					Tag: "k:1",
					Why: "all",
				},
				{
					ID:  "hYBdfG8uq5yLwieFlt6d53gZnQJafi0OSkdJxS2OZx0",
					Job: 2,
					Key: keys[1],
					Tag: "k:2",
					Why: "all",
				},
			})
		})

		Convey("Some keys are missing", func() {
			keys := []*datastore.Key{
				attachTag(i1, "k:1"),
				tagKey(i1, "k:2"), // the key for a missing tag
				attachTag(i2, "k:1"),
				attachTag(i3, "k:2"),
			}

			// Mark all discovered tags.
			err := visitAndMarkTags(ctx, 1, keys, func(t *model.Tag) string {
				return "all"
			})
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()

			var marked []markedTag
			So(datastore.GetAll(ctx, queryMarkedTags(1), &marked), ShouldBeNil)
			markedKeys := make([]*datastore.Key, len(marked))
			for i, t := range marked {
				markedKeys[i] = t.Key
			}

			// Indeed marked all keys except the missing one.
			So(markedKeys, ShouldResemble, []*datastore.Key{
				keys[3],
				keys[2],
				keys[0],
			})
		})
	})
}
