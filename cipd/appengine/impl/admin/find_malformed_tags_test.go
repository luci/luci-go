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

	"go.chromium.org/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFixMalformedTags(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, admin := SetupTest()

		iid := strings.Repeat("a", 40)
		instanceKey := func(pkg string) *datastore.Key {
			return datastore.KeyForObj(ctx, &model.Instance{
				InstanceID: iid,
				Package:    model.PackageKey(ctx, pkg),
			})
		}

		allTags := func(pkg string) (tags []string) {
			t := []model.Tag{}
			q := datastore.NewQuery("InstanceTag").Ancestor(model.PackageKey(ctx, pkg))
			So(datastore.GetAll(ctx, q, &t), ShouldBeNil)
			for _, e := range t {
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
		So(datastore.Put(ctx, tags), ShouldBeNil)

		// Make sure allTags actually works too.
		So(allTags("a"), ShouldResemble, []string{"good_a:tag", "bad_a:", "bad_a:fixable\n"})

		jobID, err := RunMapper(ctx, admin, &api.JobConfig{
			Kind: api.MapperKind_FIND_MALFORMED_TAGS,
		})
		So(err, ShouldBeNil)

		// Verify all bad tags (and only them) were marked.
		var marked []markedTag
		So(datastore.GetAll(ctx, queryMarkedTags(jobID), &marked), ShouldBeNil)
		var badTags []string
		for _, t := range marked {
			badTags = append(badTags, t.Tag)
		}
		So(badTags, ShouldResemble, []string{
			"bad_c:fixable\n",
			"bad_b:",
			"bad_a:fixable\n",
			"bad_a:",
		})

		// Fix fixable tags and delete unfixable.
		report, err := fixMarkedTags(ctx, jobID)
		So(err, ShouldBeNil)

		sort.Slice(report, func(i, j int) bool {
			return report[i].BrokenTag < report[j].BrokenTag
		})
		So(report, ShouldResemble, []*api.TagFixReport_Tag{
			{Pkg: "a", Instance: iid, BrokenTag: "bad_a:", FixedTag: ""},
			{Pkg: "a", Instance: iid, BrokenTag: "bad_a:fixable\n", FixedTag: "bad_a:fixable"},
			{Pkg: "b", Instance: iid, BrokenTag: "bad_b:", FixedTag: ""},
			{Pkg: "c", Instance: iid, BrokenTag: "bad_c:fixable\n", FixedTag: "bad_c:fixable"},
		})

		// Verify the changes actually landed in the datastore.
		So(allTags("a"), ShouldResemble, []string{"bad_a:fixable", "good_a:tag"})
		So(allTags("b"), ShouldResemble, []string{"good_b:tag"})
		So(allTags("c"), ShouldResemble, []string{"bad_c:fixable"})
	})
}
