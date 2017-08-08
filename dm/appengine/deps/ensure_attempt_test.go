// Copyright 2015 The LUCI Authors.
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

package deps

import (
	"testing"

	ds "go.chromium.org/gae/service/datastore"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEnsureAttempt(t *testing.T) {
	t.Parallel()

	Convey("EnsureGraphData (Ensuring attempts)", t, func() {
		ttest, c, _, s := testSetup()

		Convey("bad", func() {
			Convey("no quest", func() {
				_, err := s.EnsureGraphData(writer(c), &dm.EnsureGraphDataReq{
					RawAttempts: dm.NewAttemptList(map[string][]uint32{"quest": {1}}),
				})
				So(err, ShouldBeRPCInvalidArgument,
					`cannot create attempts for absent quest "quest"`)
			})
			Convey("mismatched quest", func() {
				_, err := s.EnsureGraphData(writer(c), &dm.EnsureGraphDataReq{
					Quest: []*dm.Quest_Desc{dm.NewQuestDesc("fakeDistributor", "{}", "{}", nil)},
				})
				So(err, ShouldErrLike, "mismatched quest_attempt v. quest lengths")
			})
			Convey("no auth", func() {
				desc := dm.NewQuestDesc("fakeDistributor", `{"hi": "there"}`, "{}", nil)
				_, err := s.EnsureGraphData(c, &dm.EnsureGraphDataReq{
					Quest:        []*dm.Quest_Desc{desc},
					QuestAttempt: []*dm.AttemptList_Nums{{Nums: []uint32{1}}},
				})
				So(err, ShouldBeRPCPermissionDenied, `not authorized`)
			})
		})

		Convey("good", func() {
			desc := dm.NewQuestDesc("fakeDistributor", `{"hi": "there"}`, "{}", nil)
			So(desc.Normalize(), ShouldBeNil)
			q := model.NewQuest(c, desc)
			rsp, err := s.EnsureGraphData(writer(c), &dm.EnsureGraphDataReq{
				Quest:        []*dm.Quest_Desc{desc},
				QuestAttempt: []*dm.AttemptList_Nums{{Nums: []uint32{1}}},
			})
			So(err, ShouldBeNil)
			So(rsp.Accepted, ShouldBeTrue)
			ttest.Drain(c)
			So(ds.Get(c, &model.Attempt{ID: *dm.NewAttemptID(q.ID, 1)}), ShouldBeNil)
		})

	})
}
