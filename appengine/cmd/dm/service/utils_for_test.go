// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"
	"sort"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func getService() *DungeonMaster {
	s := &DungeonMaster{}
	s.TestMode = true
	return s
}

func mkAid(qid string, num uint32) *types.AttemptID {
	return &types.AttemptID{QuestID: qid, AttemptNum: num}
}

func mkQuest(c context.Context, name string) string {
	qsts, err := getService().EnsureQuests(c, &EnsureQuestsReq{
		[]*model.QuestDescriptor{{
			Distributor: "foof",
			Payload:     []byte(fmt.Sprintf(`{"name": "%s"}`, name)),
		}},
	})
	So(err, ShouldBeNil)
	return qsts.QuestIDs[0]
}

func execute(c context.Context, aid *types.AttemptID) {
	// takes an NeedsExecution attempt, and moves it to Executing
	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		ds := datastore.Get(c)
		atmpt := &model.Attempt{AttemptID: *aid}
		So(ds.Get(atmpt), ShouldBeNil)

		atmpt.CurExecution++
		atmpt.State.MustEvolve(attempt.Executing)

		So(ds.PutMulti([]interface{}{atmpt, &model.Execution{
			ID:           atmpt.CurExecution,
			Attempt:      ds.KeyForObj(atmpt),
			ExecutionKey: []byte("sekret"),
		}}), ShouldBeNil)
		return nil
	}, nil)
	So(err, ShouldBeNil)
}

func depOn(c context.Context, from *types.AttemptID, to ...*types.AttemptID) {
	rsp, err := getService().AddDeps(c, &AddDepsReq{*from, to, []byte("sekret")})
	So(err, ShouldBeNil)
	So(rsp.ShouldHalt, ShouldBeTrue)
}

func mkDisplayDeps(from *types.AttemptID, to ...*types.AttemptID) *display.DepsFromAttempt {
	sort.Sort(types.AttemptIDSlice(to))
	ret := &display.DepsFromAttempt{From: *from}
	for _, t := range to {
		ret.To.Merge(&display.QuestAttempts{
			QuestID: t.QuestID, Attempts: types.U32s{t.AttemptNum},
		})
	}
	return ret
}
