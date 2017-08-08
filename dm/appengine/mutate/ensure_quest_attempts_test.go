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

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/tumble"

	//. "go.chromium.org/luci/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEnsureQuestAttempts(t *testing.T) {
	t.Parallel()

	Convey("EnsureQuestAttempts", t, func() {
		ttest := &tumble.Testing{}
		c := ttest.Context()

		desc := dm.NewQuestDesc("distributor", `{"data":"yes"}`, "{}", nil)
		So(desc.Normalize(), ShouldBeNil)
		qst := model.NewQuest(c, desc)

		eqa := EnsureQuestAttempts{qst, []uint32{1, 2, 3, 4}, false}

		Convey("root", func() {
			So(eqa.Root(c), ShouldResemble, ds.MakeKey(c, "Quest", qst.ID))
		})

		Convey("quest dne", func() {
			muts, err := eqa.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldResemble, []tumble.Mutation{
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 1)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 2)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 3)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 4)},
			})
		})

		Convey("tail recursion", func() {
			eqa.AIDs = append(eqa.AIDs, []uint32{5, 6, 7, 8, 9, 10, 11, 12}...)
			muts, err := eqa.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldResemble, []tumble.Mutation{
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 1)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 2)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 3)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 4)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 5)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 6)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 7)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 8)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 9)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 10)},
				&EnsureAttempt{dm.NewAttemptID(qst.ID, 11)},
				&EnsureQuestAttempts{qst, []uint32{12}, true},
			})
		})
	})
}
