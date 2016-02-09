// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestViewAttempts(t *testing.T) {
	t.Parallel()

	Convey("ViewAttempts", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		s := getService()

		Convey("bad", func() {
			Convey("no quest", func() {
				_, err := s.ViewAttempts(c, &ViewAttemptsReq{"foof"})
				So(err, ShouldErrLike, "no such quest")
			})
		})

		Convey("good", func() {
			qd := &model.QuestDescriptor{
				Distributor: "foof", Payload: []byte(`{"data": "yes"}`)}
			q, err := qd.NewQuest(c)
			So(err, ShouldBeNil)

			So(ds.Put(q), ShouldBeNil)

			Convey("no attempts", func() {
				rsp, err := s.ViewAttempts(c, &ViewAttemptsReq{q.ID})
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &display.Data{
					Attempts: display.AttemptSlice{},
				})
			})

			Convey("with attempts", func() {
				for i := uint32(0); i < 4; i++ {
					So(ds.Put(&model.Attempt{
						AttemptID: types.AttemptID{
							QuestID: q.ID, AttemptNum: 10 - i}}), ShouldBeNil)
				}

				ds.Testable().CatchupIndexes()

				rsp, err := s.ViewAttempts(c, &ViewAttemptsReq{q.ID})
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &display.Data{
					Attempts: display.AttemptSlice{
						&display.Attempt{ID: types.AttemptID{QuestID: q.ID, AttemptNum: 7}},
						&display.Attempt{ID: types.AttemptID{QuestID: q.ID, AttemptNum: 8}},
						&display.Attempt{ID: types.AttemptID{QuestID: q.ID, AttemptNum: 9}},
						&display.Attempt{ID: types.AttemptID{QuestID: q.ID, AttemptNum: 10}},
					},
				})

			})
		})

	})
}
