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
	"sort"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	adminpb "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

func TestFixMalformedTags(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, admin, sched := SetupTest()

		iid := strings.Repeat("a", 40)
		instanceKey := func(pkg string) *datastore.Key {
			return datastore.KeyForObj(ctx, &model.Instance{
				InstanceID: iid,
				Package:    model.PackageKey(ctx, pkg),
			})
		}

		allTags := func(pkg string) (tags []string) {
			mTag := []model.Tag{}
			q := datastore.NewQuery("InstanceTag").Ancestor(model.PackageKey(ctx, pkg))
			assert.Loosely(t, datastore.GetAll(ctx, q, &mTag), should.BeNil)
			for _, e := range mTag {
				tags = append(tags, e.Tag)
			}
			return
		}

		// Create a bunch of Tag entities manually, since AttachTags API won't allow
		// us to create invalid tags. Note that for this test the exact form of
		// entity keys is irrelevant.
		tags := []*model.Tag{
			{ID: "t1", Instance: instanceKey("a"), Tag: "good_a:tag"},
			{ID: "t2", Instance: instanceKey("a"), Tag: "bad_a:"},
			{ID: "t3", Instance: instanceKey("a"), Tag: "bad_a:fixable\n"},
			{ID: "t4", Instance: instanceKey("b"), Tag: "good_b:tag"},
			{ID: "t5", Instance: instanceKey("b"), Tag: "bad_b:"},
			{ID: "t6", Instance: instanceKey("c"), Tag: "bad_c:fixable\n"},
		}
		assert.Loosely(t, datastore.Put(ctx, tags), should.BeNil)

		// Make sure allTags actually works too.
		assert.Loosely(t, allTags("a"), should.Match([]string{"good_a:tag", "bad_a:", "bad_a:fixable\n"}))

		jobID, err := RunMapper(ctx, admin, sched, &adminpb.JobConfig{
			Kind: adminpb.MapperKind_FIND_MALFORMED_TAGS,
		})
		assert.Loosely(t, err, should.BeNil)

		// Verify all bad tags (and only them) were marked.
		var marked []markedTag
		assert.Loosely(t, datastore.GetAll(ctx, queryMarkedTags(jobID), &marked), should.BeNil)
		var badTags []string
		for _, mTag := range marked {
			badTags = append(badTags, mTag.Tag)
		}
		assert.Loosely(t, badTags, should.Match([]string{
			"bad_c:fixable\n",
			"bad_b:",
			"bad_a:fixable\n",
			"bad_a:",
		}))

		// Fix fixable tags and delete unfixable.
		report, err := fixMarkedTags(ctx, jobID)
		assert.Loosely(t, err, should.BeNil)

		sort.Slice(report, func(i, j int) bool {
			return report[i].BrokenTag < report[j].BrokenTag
		})
		assert.Loosely(t, report, should.Match([]*adminpb.TagFixReport_Tag{
			{Pkg: "a", Instance: iid, BrokenTag: "bad_a:", FixedTag: ""},
			{Pkg: "a", Instance: iid, BrokenTag: "bad_a:fixable\n", FixedTag: "bad_a:fixable"},
			{Pkg: "b", Instance: iid, BrokenTag: "bad_b:", FixedTag: ""},
			{Pkg: "c", Instance: iid, BrokenTag: "bad_c:fixable\n", FixedTag: "bad_c:fixable"},
		}))

		// Verify the changes actually landed in the datastore.
		assert.Loosely(t, allTags("a"), should.Match([]string{"bad_a:fixable", "good_a:tag"}))
		assert.Loosely(t, allTags("b"), should.Match([]string{"good_b:tag"}))
		assert.Loosely(t, allTags("c"), should.Match([]string{"bad_c:fixable"}))
	})
}
