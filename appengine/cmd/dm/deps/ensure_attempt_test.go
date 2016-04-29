// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEnsureAttempt(t *testing.T) {
	t.Parallel()

	Convey("EnsureGraphData (Ensuring attempts)", t, func() {
		ttest := &tumble.Testing{}
		c := ttest.Context()
		ds := datastore.Get(c)
		s := &deps{}

		Convey("bad", func() {
			Convey("no quest", func() {
				_, err := s.EnsureGraphData(c, &dm.EnsureGraphDataReq{
					Attempts: dm.NewAttemptList(map[string][]uint32{"quest": {1}}),
				})
				So(err, ShouldBeRPCInvalidArgument,
					`cannot create attempts for absent quest "quest"`)
			})
			Convey("mismatched quest", func() {
				_, err := s.EnsureGraphData(c, &dm.EnsureGraphDataReq{
					Quest:    []*dm.Quest_Desc{dm.NewQuestDesc("foof", "{}")},
					Attempts: dm.NewAttemptList(map[string][]uint32{"quest": {1}}),
				})
				So(err, ShouldErrLike, "must have a matching Attempts entry")
			})
		})

		Convey("good", func() {
			desc := dm.NewQuestDesc("swarming", `{"hi": "there"}`)
			q, err := model.NewQuest(c, desc)
			So(err, ShouldBeNil)
			rsp, err := s.EnsureGraphData(c, &dm.EnsureGraphDataReq{
				Quest:    []*dm.Quest_Desc{desc},
				Attempts: dm.NewAttemptList(map[string][]uint32{q.ID: {1}}),
			})
			So(err, ShouldBeNil)
			So(rsp.Accepted, ShouldBeTrue)
			ttest.Drain(c)
			So(ds.Get(&model.Attempt{ID: *dm.NewAttemptID(q.ID, 1)}), ShouldBeNil)
		})

	})
}
