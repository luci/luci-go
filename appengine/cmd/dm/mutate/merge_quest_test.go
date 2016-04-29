// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMergeQuest(t *testing.T) {
	t.Parallel()

	Convey("MergeQuest", t, func() {
		ttest := &tumble.Testing{}
		c := ttest.Context()
		ds := datastore.Get(c)

		desc := dm.NewQuestDesc("distributor", `{"data":"yes"}`)
		qst, err := model.NewQuest(c, desc)
		So(err, ShouldBeNil)
		qst.BuiltBy = append(qst.BuiltBy, *dm.NewTemplateSpec("a", "b", "c", "d"))

		mq := &MergeQuest{qst}

		Convey("root", func() {
			So(mq.Root(c), ShouldResemble, ds.MakeKey("Quest", qst.ID))
		})

		Convey("quest doesn't exist", func() {
			muts, err := mq.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldBeEmpty)

			q := &model.Quest{ID: qst.ID}
			So(ds.Get(q), ShouldBeNil)
			So(q, ShouldResemble, qst)
		})

		Convey("assuming it exists", func() {
			So(ds.Put(qst), ShouldBeNil)
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
				So(ds.Get(q), ShouldBeNil)
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
