// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestViewQuests(t *testing.T) {
	t.Parallel()

	Convey("ViewQuests", t, func() {
		c := memory.Use(context.Background())
		c, clk := testclock.UseTime(c, testclock.TestTimeUTC.Round(time.Millisecond))
		ds := datastore.Get(c)
		s := DungeonMaster{}

		Convey("bad", func() {
			Convey("impossible boundaries", func() {
				_, err := s.viewQuestsInternal(c, &ViewQuestsReq{From: "b", To: "a"})
				So(err, ShouldErrLike, "inverted boundaries")
			})
		})

		Convey("good", func() {
			Convey("no quests", func() {
				rsp, err := s.viewQuestsInternal(c, &ViewQuestsReq{From: "a", To: "b"})
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleV, &display.Data{Quests: display.QuestSlice{}})
			})

			Convey("some quests", func() {
				qd := &model.QuestDescriptor{Distributor: "foof", Payload: []byte(`{"data": "yes"}`)}
				q, err := qd.NewQuest(c)
				So(err, ShouldBeNil)
				So(ds.Put(q), ShouldBeNil)

				ds.Testable().CatchupIndexes()

				rsp, err := s.viewQuestsInternal(c, &ViewQuestsReq{})
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleV, &display.Data{Quests: display.QuestSlice{
					&display.Quest{ID: "dA4ML2hPh3dgKJKz0hTKL32806Kh4UB18VUXwC-EZPs",
						Distributor: "foof",
						Created:     clk.Now(),
						Payload:     `{"data":"yes"}`},
				}})
			})

			Convey("more than limit", func() {
				for i := 0; i < ViewQuestsLimit+1; i++ {
					qd := &model.QuestDescriptor{Distributor: "foof", Payload: []byte(fmt.Sprintf(`{"data": %d}`, i))}
					q, err := qd.NewQuest(c)
					So(err, ShouldBeNil)
					So(ds.Put(q), ShouldBeNil)
				}
				ds.Testable().CatchupIndexes()

				rsp, err := s.viewQuestsInternal(c, &ViewQuestsReq{})
				So(err, ShouldBeNil)
				So(len(rsp.Quests), ShouldEqual, ViewQuestsLimit)
				So(rsp.More, ShouldBeTrue)
			})

			Convey("with attempts", func() {
				qd := &model.QuestDescriptor{Distributor: "foof", Payload: []byte(`{"data": "yes"}`)}
				q, err := qd.NewQuest(c)
				So(err, ShouldBeNil)
				So(ds.Put(q), ShouldBeNil)
				So(ds.Put(&model.Attempt{AttemptID: types.AttemptID{QuestID: q.ID, AttemptNum: 1}}), ShouldBeNil)
				ds.Testable().CatchupIndexes()

				rsp, err := s.viewQuestsInternal(c, &ViewQuestsReq{WithAttempts: true})
				So(err, ShouldBeNil)
				So(len(rsp.Quests), ShouldEqual, 1)
				So(len(rsp.Attempts), ShouldEqual, 1)
			})

		})

	})
}
