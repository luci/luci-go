// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	. "github.com/luci/luci-go/common/testing/assertions"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAddDeps(t *testing.T) {
	t.Parallel()

	Convey("EnsureGraphData (Adding deps)", t, func() {
		_, c, _, s := testSetup()
		c = writer(c)
		ds := datastore.Get(c)

		a := &model.Attempt{ID: *dm.NewAttemptID("quest", 1)}
		a.CurExecution = 1
		a.State = dm.Attempt_EXECUTING
		ak := ds.KeyForObj(a)

		e := &model.Execution{
			ID: 1, Attempt: ak, Token: []byte("key"),
			State: dm.Execution_RUNNING}

		toQuestDesc := &dm.Quest_Desc{
			DistributorConfigName: "fakeDistributor",
			Parameters:            `{"data":"yes"}`,
			DistributorParameters: `{}`,
		}
		So(toQuestDesc.Normalize(), ShouldBeNil)
		toQuest := model.NewQuest(c, toQuestDesc)
		to := &model.Attempt{ID: *dm.NewAttemptID(toQuest.ID, 1)}
		fwd := &model.FwdDep{Depender: ak, Dependee: to.ID}

		req := &dm.EnsureGraphDataReq{
			ForExecution: &dm.Execution_Auth{
				Id:    dm.NewExecutionID(a.ID.Quest, a.ID.Id, 1),
				Token: []byte("key"),
			},
			RawAttempts: dm.NewAttemptList(map[string][]uint32{
				to.ID.Quest: {to.ID.Id},
			}),
		}
		So(req.Normalize(), ShouldBeNil)

		Convey("Bad", func() {
			Convey("No such originating attempt", func() {
				_, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeRPCUnauthenticated)
			})

			Convey("No such destination quest", func() {
				So(ds.Put(a, e), ShouldBeNil)

				_, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeRPCInvalidArgument, `cannot create attempts for absent quest "-asJkTOx8ORdkGZsg7Bc-w2Z_0FIB4vgD1afzInkwNE"`)
			})
		})

		Convey("Good", func() {
			So(ds.Put(a, e, toQuest), ShouldBeNil)

			Convey("deps already exist", func() {
				So(ds.Put(fwd, to), ShouldBeNil)

				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				rsp.Result.PurgeTimestamps()
				So(rsp, ShouldResemble, &dm.EnsureGraphDataRsp{
					Accepted: true,
					Result: &dm.GraphData{Quests: map[string]*dm.Quest{
						toQuest.ID: {
							Data: &dm.Quest_Data{
								Desc:    toQuestDesc,
								BuiltBy: []*dm.Quest_TemplateSpec{},
							},
							Attempts: map[uint32]*dm.Attempt{1: dm.NewAttemptScheduling()},
						},
					}},
				})
			})

			Convey("deps already done", func() {
				to.State = dm.Attempt_FINISHED
				to.Result.Data = dm.NewJsonResult(`{"done":true}`)
				to.Result.Data.Object = ""
				ar := &model.AttemptResult{
					Attempt: ds.KeyForObj(to),
					Data:    *dm.NewJsonResult(`{"done":true}`)}
				So(ds.Put(to, ar), ShouldBeNil)

				req.Include.Attempt.Result = true
				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				rsp.Result.PurgeTimestamps()
				So(rsp, ShouldResemble, &dm.EnsureGraphDataRsp{
					Accepted: true,
					Result: &dm.GraphData{Quests: map[string]*dm.Quest{
						toQuest.ID: {
							Data: &dm.Quest_Data{
								Desc:    toQuestDesc,
								BuiltBy: []*dm.Quest_TemplateSpec{},
							},
							Attempts: map[uint32]*dm.Attempt{
								1: dm.NewAttemptFinished(dm.NewJsonResult(`{"done":true}`))},
						},
					}},
				})

				So(ds.Get(fwd), ShouldBeNil)
			})

			Convey("adding new deps", func() {
				So(ds.Put(&model.Quest{ID: "to"}), ShouldBeNil)

				rsp, err := s.EnsureGraphData(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &dm.EnsureGraphDataRsp{ShouldHalt: true})

				So(ds.Get(fwd), ShouldBeNil)
				So(ds.Get(a), ShouldBeNil)
				So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
				So(ds.Get(e), ShouldBeNil)
				So(e.State, ShouldEqual, dm.Execution_STOPPING)
			})
		})
	})
}
