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
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEnsureQuests(t *testing.T) {
	t.Parallel()

	desc := func(payload string) *dm.Quest_Desc {
		return &dm.Quest_Desc{
			DistributorConfigName: "fakeDistributor",
			Parameters:            payload,
			DistributorParameters: "{}",
		}
	}

	Convey("EnsureGraphData (Ensure Quests)", t, func() {
		ttest, c, _, s := testSetup()
		c = writer(c)
		clk := clock.Get(c).(testclock.TestClock)

		Convey("bad", func() {
			// TODO(riannucci): restore this once moving to the new distributor scheme
			/*
				Convey("missing distributor", func() {
					_, err := s.EnsureQuests(c, &dm.EnsureQuestsReq{
						ToEnsure: []*dm.Quest_Desc{desc("{}")},
					})
					So(err, ShouldErrLike, "unknown distributors")
				})
			*/
		})

		Convey("good", func() {
			// TODO(riannucci): add fakeDistributor distributor configuration

			qd := desc(`{"data": "yes"}`)
			So(qd.Normalize(), ShouldBeNil)
			q := model.NewQuest(c, qd)

			qd2 := desc(`{"data": "way yes"}`)
			So(qd2.Normalize(), ShouldBeNil)
			q2 := model.NewQuest(c, qd2)

			req := &dm.EnsureGraphDataReq{
				Quest: []*dm.Quest_Desc{qd, qd2},
				QuestAttempt: []*dm.AttemptList_Nums{
					{Nums: []uint32{1}},
					{Nums: []uint32{2}},
				}}

			Convey("0/2 exist", func() {
				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				So(rsp.Result.Quests, ShouldResemble, map[string]*dm.Quest{
					q.ID:  {DNE: true, Attempts: map[uint32]*dm.Attempt{1: {DNE: true}}},
					q2.ID: {DNE: true, Attempts: map[uint32]*dm.Attempt{2: {DNE: true}}},
				})
				ttest.Drain(c)
				rsp, err = s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				rsp.Result.PurgeTimestamps()
				So(rsp.Result.Quests, ShouldResemble, map[string]*dm.Quest{
					q.ID: {
						Data: &dm.Quest_Data{
							Desc:    qd,
							BuiltBy: []*dm.Quest_TemplateSpec{},
						},
						Attempts: map[uint32]*dm.Attempt{1: dm.NewAttemptExecuting(1)}},
					q2.ID: {
						Data: &dm.Quest_Data{
							Desc:    qd2,
							BuiltBy: []*dm.Quest_TemplateSpec{},
						},
						Attempts: map[uint32]*dm.Attempt{2: dm.NewAttemptExecuting(1)}},
				})
			})

			Convey("1/2 exist", func() {
				So(ds.Put(c, q), ShouldBeNil)

				clk.Add(time.Minute)

				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				rsp.Result.PurgeTimestamps()
				So(rsp.Result.Quests, ShouldResemble, map[string]*dm.Quest{
					q.ID: {
						Data: &dm.Quest_Data{
							Desc:    qd,
							BuiltBy: []*dm.Quest_TemplateSpec{},
						},
						Attempts: map[uint32]*dm.Attempt{1: {DNE: true}},
					},
					q2.ID: {DNE: true, Attempts: map[uint32]*dm.Attempt{2: {DNE: true}}},
				})
				now := clk.Now()
				ttest.Drain(c)

				qNew := &model.Quest{ID: q.ID}
				So(ds.Get(c, qNew), ShouldBeNil)
				So(qNew.Created, ShouldResemble, q.Created)

				q2New := &model.Quest{ID: q2.ID}
				So(ds.Get(c, q2New), ShouldBeNil)
				So(q2New.Created, ShouldResemble, now.Round(time.Microsecond))
			})

			Convey("all exist", func() {
				So(ds.Put(c, q), ShouldBeNil)
				So(ds.Put(c, q2), ShouldBeNil)

				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				rsp.Result.PurgeTimestamps()
				So(rsp.Result.Quests, ShouldResemble, map[string]*dm.Quest{
					q.ID: {
						Data: &dm.Quest_Data{
							Desc:    qd,
							BuiltBy: []*dm.Quest_TemplateSpec{},
						},
						Attempts: map[uint32]*dm.Attempt{1: {DNE: true}}},
					q2.ID: {
						Data: &dm.Quest_Data{
							Desc:    qd2,
							BuiltBy: []*dm.Quest_TemplateSpec{},
						},
						Attempts: map[uint32]*dm.Attempt{2: {DNE: true}}},
				})

				qNew := &model.Quest{ID: q.ID}
				So(ds.Get(c, qNew), ShouldBeNil)
				So(qNew.Created, ShouldResemble, q.Created)
			})
		})
	})
}
