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

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFindMalformedTags(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, admin := SetupTest()

		instanceKey := func(pkg string) *datastore.Key {
			return datastore.KeyForObj(ctx, &model.Instance{
				InstanceID: strings.Repeat("a", 40),
				Package:    model.PackageKey(ctx, pkg),
			})
		}

		// Create a bunch of Tag entities manually, since AttachTags API won't allow
		// us to create invalid tags. Note that for this test the exact form of
		// entity keys is irrelevant.
		tags := []*model.Tag{
			{ID: "t1", Instance: instanceKey("a"), Tag: "good1:tag"},
			{ID: "t2", Instance: instanceKey("a"), Tag: "bad1:"},
			{ID: "t3", Instance: instanceKey("b"), Tag: "good2:tag"},
			{ID: "t4", Instance: instanceKey("b"), Tag: "bad2:"},
		}
		So(datastore.Put(ctx, tags), ShouldBeNil)

		jobID, err := RunMapper(ctx, admin, &api.JobConfig{
			Kind: api.MapperKind_FIND_MALFORMED_TAGS,
		})
		So(err, ShouldBeNil)

		// Verify all bad tags (and only them) were marked.
		var marked []markedTag
		So(datastore.GetAll(ctx, queryMarkedTags(jobID), &marked), ShouldBeNil)
		So(marked, ShouldHaveLength, 2)
		So(marked[0].Tag, ShouldEqual, "bad2:")
		So(marked[0].Key, ShouldResemble, datastore.KeyForObj(ctx, tags[3]))
		So(marked[1].Tag, ShouldEqual, "bad1:")
		So(marked[1].Key, ShouldResemble, datastore.KeyForObj(ctx, tags[1]))
	})
}
