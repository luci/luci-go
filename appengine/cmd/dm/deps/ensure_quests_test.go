// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestEnsureQuests(t *testing.T) {
	t.Parallel()

	desc := func(payload string) *dm.Quest_Desc {
		return &dm.Quest_Desc{
			DistributorConfigName: "foof",
			JsonPayload:           payload,
		}
	}

	Convey("EnsureQuests", t, func() {
		c := memory.Use(context.Background())
		c, clk := testclock.UseTime(c, testclock.TestTimeUTC.Round(time.Millisecond))
		ds := datastore.Get(c)
		s := &deps{}

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
			// TODO(riannucci): add foof distributor configuration

			qd := desc(`{"data": "yes"}`)
			q, err := model.NewQuest(c, qd)
			So(err, ShouldBeNil)

			qd2 := desc(`{"data": "way yes"}`)
			q2, err := model.NewQuest(c, qd2)
			So(err, ShouldBeNil)

			req := &dm.EnsureQuestsReq{ToEnsure: []*dm.Quest_Desc{qd, qd2}}

			Convey("0/2 exist", func() {
				rsp, err := s.EnsureQuests(c, req)
				So(err, ShouldBeNil)
				So(rsp.QuestIds, ShouldResemble, []*dm.Quest_ID{{Id: q.ID}, {Id: q2.ID}})
			})

			Convey("1/2 exist", func() {
				So(ds.Put(q), ShouldBeNil)

				clk.Add(time.Minute)

				rsp, err := s.EnsureQuests(c, req)
				So(err, ShouldBeNil)
				So(rsp.QuestIds, ShouldResemble, []*dm.Quest_ID{{Id: q.ID}, {Id: q2.ID}})

				qNew := &model.Quest{ID: q.ID}
				So(ds.Get(qNew), ShouldBeNil)
				So(qNew.Created, ShouldResemble, q.Created)

				q2New := &model.Quest{ID: q2.ID}
				So(ds.Get(q2New), ShouldBeNil)
				So(q2New.Created, ShouldResemble, clk.Now().Round(time.Microsecond))

			})

			Convey("all exist", func() {
				So(ds.Put(q), ShouldBeNil)

				rsp, err := s.EnsureQuests(c, req)
				So(err, ShouldBeNil)
				So(rsp.QuestIds, ShouldResemble, []*dm.Quest_ID{{Id: q.ID}, {Id: q2.ID}})

				qNew := &model.Quest{ID: q.ID}
				So(ds.Get(qNew), ShouldBeNil)
				So(qNew.Created, ShouldResemble, q.Created)
			})
		})
	})
}
