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

package model

import (
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
)

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		iid := func(i int) string {
			ch := string([]byte{'0' + byte(i)})
			return strings.Repeat(ch, 40)
		}

		ids := func(inst []*Instance) []string {
			out := make([]string, len(inst))
			for i, ent := range inst {
				out[i] = ent.InstanceID
			}
			return out
		}

		put := func(when int, iid string, tags ...string) {
			inst := &Instance{
				InstanceID:   iid,
				Package:      PackageKey(ctx, "pkg"),
				RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
			}
			ents := make([]*Tag, len(tags))
			for i, t := range tags {
				ents[i] = &Tag{
					ID:           TagID(common.MustParseInstanceTag(t)),
					Instance:     datastore.KeyForObj(ctx, inst),
					Tag:          t,
					RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
				}
			}
			assert.Loosely(t, datastore.Put(ctx, inst, ents), should.BeNil)
		}

		t.Run("Search by one tag works", func(t *ftt.Test) {
			expectedIIDs := make([]string, 10)
			for i := range 10 {
				put(i, iid(i), "k:v0")
				expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
			}

			t.Run("Empty", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "missing"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, out, should.HaveLength(0))
			})

			t.Run("No pagination", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match(expectedIIDs))
			})

			t.Run("With pagination", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 6, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.NotBeNil)
				assert.Loosely(t, ids(out), should.Match(expectedIIDs[:6]))

				out, cur, err = SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 6, cur)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match(expectedIIDs[6:10]))
			})

			t.Run("Handle missing instances", func(t *ftt.Test) {
				datastore.Delete(ctx, &Instance{
					InstanceID: iid(5),
					Package:    PackageKey(ctx, "pkg"),
				})

				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match(append(append([]string(nil),
					expectedIIDs[:4]...),
					expectedIIDs[5:]...)))
			})
		})

		t.Run("Search by many tags works", func(t *ftt.Test) {
			expectedIIDs := make([]string, 10)
			for i := range 10 {
				tags := []string{"k:v0"}
				if i%2 == 0 {
					tags = append(tags, "k:v1")
				}
				if i%3 == 0 {
					tags = append(tags, "k:v2")
				}
				put(i, iid(i), tags...)
				expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
			}

			t.Run("Empty result", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "missing"},
					{Key: "k", Value: "v2"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, out, should.HaveLength(0))
			})

			t.Run("No pagination", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v1"}, // dup
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match([]string{
					iid(8), iid(6), iid(4), iid(2), iid(0),
				}))
			})

			t.Run("With pagination", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
				}, 3, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.NotBeNil)
				assert.Loosely(t, ids(out), should.Match([]string{
					iid(8), iid(6), iid(4),
				}))

				out, cur, err = SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
				}, 3, cur)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match([]string{
					iid(2), iid(0),
				}))
			})

			t.Run("More filters", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v2"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match([]string{
					iid(6), iid(0),
				}))
			})

			t.Run("Tags order doesn't matter", func(t *ftt.Test) {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v2"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v0"},
				}, 11, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cur, should.BeNil)
				assert.Loosely(t, ids(out), should.Match([]string{
					iid(6), iid(0),
				}))
			})
		})
	})
}
