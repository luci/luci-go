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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
)

func TestVisitAndMarkTags(t *testing.T) {
	t.Parallel()

	ftt.Run("With a bunch of instances", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		makeInst := func(pkg, iid string) *model.Instance {
			i := &model.Instance{
				InstanceID: iid,
				Package:    model.PackageKey(ctx, pkg),
			}
			assert.Loosely(t, datastore.Put(ctx, i), should.BeNil)
			return i
		}

		tagKey := func(i *model.Instance, tag string) *datastore.Key {
			parsed, err := common.ParseInstanceTag(tag)
			assert.Loosely(t, err, should.BeNil)
			return datastore.KeyForObj(ctx, &model.Tag{
				ID:       model.TagID(parsed),
				Instance: datastore.KeyForObj(ctx, i),
			})
		}

		attachTag := func(i *model.Instance, tag string) *datastore.Key {
			parsed, err := common.ParseInstanceTag(tag)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, model.AttachTags(ctx, i, []*api.Tag{parsed}), should.BeNil)
			return tagKey(i, tag)
		}

		// Create a bunch of instances to attach tags to.
		i1 := makeInst("a/b", strings.Repeat("a", 40))
		i2 := makeInst("a/b", strings.Repeat("b", 40))
		i3 := makeInst("c", strings.Repeat("a", 40))

		t.Run("Happy path", func(t *ftt.Test) {
			// Actually create tags.
			keys := []*datastore.Key{
				attachTag(i1, "k:1"),
				attachTag(i1, "k:2"),
				attachTag(i2, "k:1"),
				attachTag(i3, "k:2"),
			}

			// Mark all k:1 tags.
			err := visitAndMarkTags(ctx, 1, keys, func(mTag *model.Tag) string {
				if mTag.Tag == "k:1" {
					return "why not?"
				}
				return ""
			})
			assert.Loosely(t, err, should.BeNil)

			// Mark all tags (in a different job).
			err = visitAndMarkTags(ctx, 2, keys, func(mTag *model.Tag) string {
				return "all"
			})
			assert.Loosely(t, err, should.BeNil)

			datastore.GetTestable(ctx).CatchupIndexes()

			// Correctly have only 1 tags (and only them) in job #1 output!
			var marked []markedTag
			assert.Loosely(t, datastore.GetAll(ctx, queryMarkedTags(1), &marked), should.BeNil)
			assert.Loosely(t, marked, should.Resemble([]markedTag{
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
			}))

			// Job #2 has collected all tags. And IDs are different from job #1.
			marked = nil
			assert.Loosely(t, datastore.GetAll(ctx, queryMarkedTags(2), &marked), should.BeNil)
			assert.Loosely(t, marked, should.Resemble([]markedTag{
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
			}))
		})

		t.Run("Some keys are missing", func(t *ftt.Test) {
			keys := []*datastore.Key{
				attachTag(i1, "k:1"),
				tagKey(i1, "k:2"), // the key for a missing tag
				attachTag(i2, "k:1"),
				attachTag(i3, "k:2"),
			}

			// Mark all discovered tags.
			err := visitAndMarkTags(ctx, 1, keys, func(mTag *model.Tag) string {
				return "all"
			})
			assert.Loosely(t, err, should.BeNil)

			datastore.GetTestable(ctx).CatchupIndexes()

			var marked []markedTag
			assert.Loosely(t, datastore.GetAll(ctx, queryMarkedTags(1), &marked), should.BeNil)
			markedKeys := make([]*datastore.Key, len(marked))
			for i, mTag := range marked {
				markedKeys[i] = mTag.Key
			}

			// Indeed marked all keys except the missing one.
			assert.Loosely(t, markedKeys, should.Resemble([]*datastore.Key{
				keys[3],
				keys[2],
				keys[0],
			}))
		})
	})
}
