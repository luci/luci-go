// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/cryptorand"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func ensureQuest(c context.Context, name string, aids ...uint32) string {
	desc := &dm.Quest_Desc{
		DistributorConfigName: "foof",
		JsonPayload:           fmt.Sprintf(`{"name": "%s"}`, name),
	}
	q, err := model.NewQuest(c, desc)
	So(err, ShouldBeNil)
	qsts, err := newDecoratedDeps().EnsureGraphData(c, &dm.EnsureGraphDataReq{
		Quest:    []*dm.Quest_Desc{desc},
		Attempts: dm.NewAttemptList(map[string][]uint32{q.ID: aids}),
	})
	So(err, ShouldBeNil)
	for qid := range qsts.Result.Quests {
		So(qid, ShouldEqual, q.ID)
		return qid
	}
	panic("impossible")
}

func mkToken(c context.Context) []byte {
	rtok := make([]byte, 32)
	_, err := cryptorand.Read(c, rtok)
	if err != nil {
		panic(err)
	}
	return rtok
}

func execute(c context.Context, aid *dm.Attempt_ID) *dm.Execution_Auth {
	// takes an NeedsExecution attempt, and moves it to Executing
	rtok := mkToken(c)
	ret := &dm.Execution_Auth{Token: rtok}

	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		ds := datastore.Get(c)
		atmpt := &model.Attempt{ID: *aid}
		if err := ds.Get(atmpt); err != nil {
			panic(err)
		}

		atmpt.CurExecution++
		atmpt.MustModifyState(c, dm.Attempt_EXECUTING)

		ret.Id = dm.NewExecutionID(atmpt.ID.Quest, atmpt.ID.Id, atmpt.CurExecution)

		So(ds.PutMulti([]interface{}{atmpt, &model.Execution{
			ID:      atmpt.CurExecution,
			Created: clock.Now(c),
			State:   dm.Execution_SCHEDULED,
			Attempt: ds.KeyForObj(atmpt),
			Token:   rtok,
		}}), ShouldBeNil)
		return nil
	}, nil)
	So(err, ShouldBeNil)
	return ret
}

func activate(c context.Context, auth *dm.Execution_Auth) *dm.Execution_Auth {
	newTok := mkToken(c)
	_, err := newDecoratedDeps().ActivateExecution(c, &dm.ActivateExecutionReq{
		Auth: auth, ExecutionToken: newTok})
	So(err, ShouldBeNil)
	return &dm.Execution_Auth{Id: auth.Id, Token: newTok}
}

func depOn(c context.Context, fromAuth *dm.Execution_Auth, to ...*dm.Attempt_ID) {
	req := &dm.EnsureGraphDataReq{
		ForExecution: fromAuth,
		Attempts:     dm.NewAttemptList(nil),
	}
	req.Attempts.AddAIDs(to...)

	rsp, err := newDecoratedDeps().EnsureGraphData(c, req)
	if err != nil {
		panic(err)
	}
	So(rsp.ShouldHalt, ShouldBeTrue)
}

func purgeTimestamps(gd *dm.GraphData) {
	for _, q := range gd.Quests {
		if q.GetData().GetCreated() != nil {
			q.Data.Created = nil
		}
		for _, a := range q.GetAttempts() {
			if a.Data != nil {
				a.Data.Created = nil
				a.Data.Modified = nil
				if ne := a.Data.GetNeedsExecution(); ne != nil {
					ne.Pending = nil
				} else if fi := a.Data.GetFinished(); fi != nil {
					fi.Expiration = nil
				}
			}
			for _, e := range a.Executions {
				if e.Data != nil {
					e.Data.Created = nil
				}
			}
		}
	}
}

func WalkShouldReturn(c context.Context, keepTimestamps ...bool) func(request interface{}, expect ...interface{}) string {
	kt := len(keepTimestamps) > 0 && keepTimestamps[0]

	normalize := func(gd *dm.GraphData) *dm.GraphData {
		data, err := proto.Marshal(gd)
		if err != nil {
			panic(err)
		}
		So(err, ShouldBeNil)
		ret := &dm.GraphData{}
		if err := proto.Unmarshal(data, ret); err != nil {
			panic(err)
		}
		if !kt {
			purgeTimestamps(ret)
		}
		return ret
	}

	return func(request interface{}, expect ...interface{}) string {
		r := request.(*dm.WalkGraphReq)
		e := expect[0].(*dm.GraphData)
		ret, err := newDecoratedDeps().WalkGraph(c, r)
		if nilExpect := ShouldErrLike(err, nil); nilExpect != "" {
			return nilExpect
		}
		return ShouldResemble(normalize(ret), e)
	}
}
