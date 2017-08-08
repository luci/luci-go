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

package mutate

import (
	"testing"

	"go.chromium.org/gae/filter/featureBreaker"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/tumble"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMergeQuest(t *testing.T) {
	t.Parallel()

	Convey("MergeQuest", t, func() {
		ttest := &tumble.Testing{}
		c := ttest.Context()

		desc := dm.NewQuestDesc("distributor", `{"data":"yes"}`, "{}", nil)
		So(desc.Normalize(), ShouldBeNil)
		qst := model.NewQuest(c, desc)
		qst.BuiltBy = append(qst.BuiltBy, *dm.NewTemplateSpec("a", "b", "c", "d"))

		mq := &MergeQuest{qst, nil}

		Convey("root", func() {
			So(mq.Root(c), ShouldResemble, ds.MakeKey(c, "Quest", qst.ID))
		})

		Convey("quest doesn't exist", func() {
			muts, err := mq.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldBeEmpty)

			q := &model.Quest{ID: qst.ID}
			So(ds.Get(c, q), ShouldBeNil)
			So(q, ShouldResemble, qst)
		})

		Convey("assuming it exists", func() {
			So(ds.Put(c, qst), ShouldBeNil)
			Convey("noop merge", func() {
				muts, err := mq.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeEmpty)
			})

			Convey("actual merge", func() {
				ttest.AdvanceTime(c)

				mq.Quest.BuiltBy.Add(*dm.NewTemplateSpec("aa", "bb", "cc", "dd"))
				muts, err := mq.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeEmpty)

				q := &model.Quest{ID: qst.ID}
				So(ds.Get(c, q), ShouldBeNil)
				So(len(q.BuiltBy), ShouldEqual, 2)
				So(q.Created, ShouldResemble, qst.Created)
			})

			Convey("datastore fail", func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(nil, "GetMulti")
				_, err := mq.RollForward(c)
				So(err, ShouldBeRPCUnknown)
			})
		})

	})
}
