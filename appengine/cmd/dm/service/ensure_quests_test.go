// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestEnsureQuests(t *testing.T) {
	t.Parallel()

	Convey("EnsureQuests", t, func() {
		c := memory.Use(context.Background())
		c, clk := testclock.UseTime(c, testclock.TestTimeUTC.Round(time.Millisecond))
		ds := datastore.Get(c)
		s := getService()

		Convey("bad", func() {
			Convey("missing distributor", func() {
				_, err := s.EnsureQuests(c, &EnsureQuestsReq{
					[]*model.QuestDescriptor{
						{Distributor: "foof", Payload: []byte("{}")},
					},
				})
				So(err, ShouldErrLike, "unknown distributors")
			})
		})

		Convey("good", func() {
			So(ds.Put(&model.Distributor{
				Name: "foof", URL: "https://foof.example.com"}), ShouldBeNil)

			qd := &model.QuestDescriptor{
				Distributor: "foof",
				Payload:     []byte(`{"data": "yes"}`),
			}
			q, err := qd.NewQuest(c)
			So(err, ShouldBeNil)

			qd2 := &model.QuestDescriptor{
				Distributor: "foof",
				Payload:     []byte(`{"data": "way yes"}`),
			}
			q2, err := qd2.NewQuest(c)
			So(err, ShouldBeNil)

			req := &EnsureQuestsReq{[]*model.QuestDescriptor{qd, qd2}}

			Convey("0/2 exist", func() {
				rsp, err := s.EnsureQuests(c, req)
				So(err, ShouldBeNil)
				So(rsp.QuestIDs, ShouldResemble, []string{q.ID, q2.ID})
			})

			Convey("1/2 exist", func() {
				So(ds.Put(q), ShouldBeNil)

				clk.Add(time.Minute)

				rsp, err := s.EnsureQuests(c, req)
				So(err, ShouldBeNil)
				So(rsp.QuestIDs, ShouldResemble, []string{q.ID, q2.ID})

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
				So(rsp.QuestIDs, ShouldResemble, []string{q.ID, q2.ID})

				qNew := &model.Quest{ID: q.ID}
				So(ds.Get(qNew), ShouldBeNil)
				So(qNew.Created, ShouldResemble, q.Created)
			})
		})
	})
}
