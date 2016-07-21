// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/distributor"
	"github.com/luci/luci-go/appengine/cmd/dm/distributor/fake"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	google_pb "github.com/luci/luci-go/common/proto/google"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type breakFwdDepLoads struct {
	datastore.RawInterface
}

func (b breakFwdDepLoads) GetMulti(keys []*datastore.Key, mg datastore.MultiMetaGetter, cb datastore.GetMultiCB) error {
	for _, k := range keys {
		if k.Kind() == "FwdDep" {
			return fmt.Errorf("Loading FwdDeps is currently broken")
		}
	}
	return b.RawInterface.GetMulti(keys, mg, cb)
}

func TestWalkGraph(t *testing.T) {
	t.Parallel()

	Convey("WalkGraph", t, func() {
		ttest, c, dist, s := testSetup()

		ds := datastore.Get(c)

		req := &dm.WalkGraphReq{
			Query: dm.AttemptListQueryL(map[string][]uint32{"quest": {1}}),
			Limit: &dm.WalkGraphReq_Limit{MaxDepth: 1},
		}
		So(req.Normalize(), ShouldBeNil)

		Convey("no read access", func() {
			_, err := s.WalkGraph(c, req)
			So(err, ShouldBeRPCUnauthenticated, "not authorized")
		})

		c = reader(c)

		Convey("no attempt", func() {
			So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
				Quests: map[string]*dm.Quest{"quest": {
					Attempts: map[uint32]*dm.Attempt{
						1: {DNE: true},
					},
				}},
			})
		})

		Convey("good", func() {
			ds.Testable().Consistent(true)

			wDesc := fake.QuestDesc("w")
			w := s.ensureQuest(c, "w", 1)
			ttest.Drain(c)

			req.Query.AttemptList = dm.NewAttemptList(
				map[string][]uint32{w: {1}})

			Convey("include nothing", func() {
				So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Attempts: map[uint32]*dm.Attempt{1: {}},
						},
					},
				})
			})

			Convey("quest dne", func() {
				req.Include.QuestData = true
				req.Limit.MaxDepth = 1
				req.Query.AttemptList = dm.NewAttemptList(
					map[string][]uint32{"noex": {1}})
				So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						"noex": {
							DNE:      true,
							Attempts: map[uint32]*dm.Attempt{1: {DNE: true}},
						},
					},
				})
			})

			Convey("no dependencies", func() {
				req.Include.AttemptData = true
				req.Include.QuestData = true
				req.Include.NumExecutions = 128
				tok := string(fake.MkToken(dm.NewExecutionID(w, 1, 1)))
				aExpect := dm.NewAttemptExecuting(1)
				aExpect.Executions = map[uint32]*dm.Execution{1: dm.NewExecutionScheduling()}
				aExpect.Executions[1].Data.DistributorInfo = &dm.Execution_Data_DistributorInfo{
					ConfigName:    "fakeDistributor",
					ConfigVersion: "testing",
					Token:         tok,
				}
				So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Data: &dm.Quest_Data{
								Desc: wDesc,
							},
							Attempts: map[uint32]*dm.Attempt{1: aExpect},
						},
					},
				})
			})

			Convey("finished", func() {
				wEx := dm.NewExecutionID(w, 1, 1)
				dist.RunTask(c, wEx, func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).Finish(
						`{"data": ["very", "yes"]}`, clock.Now(c).Add(time.Hour*24*4))
					tsk.State = []byte("distributorState")
					return nil
				})
				ttest.Drain(c)

				req.Include.AttemptData = true
				req.Include.AttemptResult = true
				req.Include.NumExecutions = 128
				req.Include.ExecutionInfoUrl = true
				data := `{"data":["very","yes"]}`
				aExpect := dm.NewAttemptFinished(time.Time{}, uint32(len(data)), data, []byte("distributorState"))
				aExpect.Data.NumExecutions = 1
				aExpect.Executions = map[uint32]*dm.Execution{
					1: dm.NewExecutionFinished("distributorState"),
				}
				tok := string(fake.MkToken(dm.NewExecutionID(w, 1, 1)))
				aExpect.Executions[1].Data.DistributorInfo = &dm.Execution_Data_DistributorInfo{
					ConfigName:    "fakeDistributor",
					ConfigVersion: "testing",
					Token:         tok,
					Url:           dist.InfoURL(distributor.Token(tok)),
				}

				So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Attempts: map[uint32]*dm.Attempt{1: aExpect},
						},
					},
				})
			})

			Convey("limited attempt results", func() {
				wEx := dm.NewExecutionID(w, 1, 1)
				dist.RunTask(c, wEx, func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).Finish(
						`{"data": ["very", "yes"]}`, clock.Now(c).Add(time.Hour*24*4))
					tsk.State = []byte("distributorState")
					return nil
				})
				ttest.Drain(c)

				req.Include.AttemptResult = true
				req.Limit.MaxDataSize = 10
				data := `{"data":["very","yes"]}`
				aExpect := dm.NewAttemptFinished(time.Time{}, uint32(len(data)), "", nil)
				aExpect.Data.NumExecutions = 1
				aExpect.Partial = &dm.Attempt_Partial{Result: dm.Attempt_Partial_DATA_SIZE_LIMIT}
				So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Attempts: map[uint32]*dm.Attempt{1: aExpect},
						},
					},
				})
			})

			Convey("attemptRange", func() {
				x := s.ensureQuest(c, "x", 1)
				ttest.Drain(c)

				wEx := dm.NewExecutionID(w, 1, 1)
				dist.RunTask(c, wEx, func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).DepOn(
						dm.NewAttemptID(x, 1), dm.NewAttemptID(x, 2), dm.NewAttemptID(x, 3),
						dm.NewAttemptID(x, 4))
					return nil
				})
				ttest.Drain(c)

				req.Limit.MaxDepth = 1
				Convey("normal", func() {
					req.Query = dm.AttemptRangeQuery(x, 2, 4)
					So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
						Quests: map[string]*dm.Quest{
							x: {Attempts: map[uint32]*dm.Attempt{2: {}, 3: {}}},
						},
					})
				})

				Convey("oob range", func() {
					req.Query = dm.AttemptRangeQuery(x, 2, 6)
					So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
						Quests: map[string]*dm.Quest{
							x: {Attempts: map[uint32]*dm.Attempt{
								2: {}, 3: {}, 4: {}, 5: {DNE: true}}},
						},
					})
				})
			})

			Convey("filtered attempt results", func() {
				x := s.ensureQuest(c, "x", 2)
				ttest.Drain(c)

				dist.RunTask(c, dm.NewExecutionID(w, 1, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).DepOn(dm.NewAttemptID(x, 1))
					tsk.State = []byte("originalState")
					return nil
				})
				ttest.Drain(c)

				exp := datastore.RoundTime(clock.Now(c).Add(time.Hour * 24 * 4))

				x1data := `{"data":["I can see this"]}`
				dist.RunTask(c, dm.NewExecutionID(x, 1, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).Finish(x1data, exp)
					tsk.State = []byte("atmpt1")
					return nil
				})

				x2data := `{"data":["nope"]}`
				dist.RunTask(c, dm.NewExecutionID(x, 2, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).Finish(x2data, exp)
					tsk.State = []byte("atmpt2")
					return nil
				})

				// This Drain does:
				//   RecordCompletion -> AckFwdDep -> ScheduleExecution
				// which attempts to load the configuration from the context, and
				// panics if it's missing.
				ttest.Drain(c)

				wEID := dm.NewExecutionID(w, 1, 2)
				wEx := model.ExecutionFromID(c, wEID)
				So(ds.Get(wEx), ShouldBeNil)

				dist.RunTask(c, wEID, func(tsk *fake.Task) error {
					So(tsk.State, ShouldResemble, distributor.PersistentState("originalState"))

					act := tsk.MustActivate(c, s)
					req.Limit.MaxDepth = 2
					req.Include.AttemptResult = true
					req.Query = dm.AttemptListQueryL(map[string][]uint32{x: nil})

					x1Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x1data)), x1data, []byte("atmpt1"))
					x1Expect.Data.NumExecutions = 1

					x2Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x2data)), "", nil)
					x2Expect.Partial = &dm.Attempt_Partial{Result: dm.Attempt_Partial_NOT_AUTHORIZED}
					x2Expect.Data.NumExecutions = 1

					So(req, act.WalkShouldReturn, &dm.GraphData{
						Quests: map[string]*dm.Quest{
							x: {Attempts: map[uint32]*dm.Attempt{
								1: x1Expect,
								2: x2Expect,
							}},
						},
					})
					return nil
				})
			})

			Convey("own attempt results", func() {
				x := s.ensureQuest(c, "x", 2)
				ttest.Drain(c)
				dist.RunTask(c, dm.NewExecutionID(w, 1, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).DepOn(dm.NewAttemptID(x, 1))
					return nil
				})
				ttest.Drain(c)

				exp := datastore.RoundTime(clock.Now(c).Add(time.Hour * 24 * 4))

				x1data := `{"data":["I can see this"]}`
				dist.RunTask(c, dm.NewExecutionID(x, 1, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).Finish(x1data, exp)
					tsk.State = []byte("state")
					return nil
				})
				ttest.Drain(c)

				dist.RunTask(c, dm.NewExecutionID(w, 1, 2), func(tsk *fake.Task) error {
					act := tsk.MustActivate(c, s)
					req.Limit.MaxDepth = 2
					req.Include.AttemptResult = true
					req.Query = dm.AttemptListQueryL(map[string][]uint32{w: {1}})

					x1Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x1data)), x1data, []byte("state"))
					x1Expect.Data.NumExecutions = 1

					w1Exepct := dm.NewAttemptExecuting(2)
					w1Exepct.Data.NumExecutions = 2

					// This filter ensures that WalkShouldReturn is using the optimized
					// path for deps traversal when starting from an authed attempt.
					c = datastore.AddRawFilters(c, func(c context.Context, ri datastore.RawInterface) datastore.RawInterface {
						return breakFwdDepLoads{ri}
					})

					So(req, act.WalkShouldReturn, &dm.GraphData{
						Quests: map[string]*dm.Quest{
							w: {Attempts: map[uint32]*dm.Attempt{1: w1Exepct}},
							x: {Attempts: map[uint32]*dm.Attempt{1: x1Expect}},
						},
					})
					return nil
				})
			})

			Convey("deps (no dest attempts)", func() {
				req.Limit.MaxDepth = 3

				x := s.ensureQuest(c, "x", 1)
				ttest.Drain(c)

				dist.RunTask(c, dm.NewExecutionID(w, 1, 1), func(tsk *fake.Task) error {
					tsk.MustActivate(c, s).DepOn(dm.NewAttemptID(x, 1), dm.NewAttemptID(x, 2))

					Convey("before tumble", func() {
						req.Include.FwdDeps = true
						// didn't run tumble, so that x|1 and x|2 don't get created.
						// don't use act.WalkShouldReturn; we want to observe the graph
						// state from
						So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
							Quests: map[string]*dm.Quest{
								w: {Attempts: map[uint32]*dm.Attempt{1: {
									FwdDeps: dm.NewAttemptList(map[string][]uint32{
										x: {2, 1},
									}),
								}}},
								x: {Attempts: map[uint32]*dm.Attempt{
									1: {FwdDeps: &dm.AttemptList{}}, // exists, but has no fwddeps
									2: {DNE: true},
								}},
							},
						})
					})
					return nil
				})

				Convey("after tumble", func() {
					ttest.Drain(c)

					Convey("deps (with dest attempts)", func() {
						req.Include.FwdDeps = true
						req.Include.BackDeps = true
						So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
							Quests: map[string]*dm.Quest{
								w: {Attempts: map[uint32]*dm.Attempt{1: {
									FwdDeps:  dm.NewAttemptList(map[string][]uint32{x: {2, 1}}),
									BackDeps: &dm.AttemptList{},
								}}},
								x: {Attempts: map[uint32]*dm.Attempt{1: {
									FwdDeps:  &dm.AttemptList{},
									BackDeps: dm.NewAttemptList(map[string][]uint32{w: {1}}),
								}, 2: {
									FwdDeps:  &dm.AttemptList{},
									BackDeps: dm.NewAttemptList(map[string][]uint32{w: {1}}),
								}}},
							},
						})
					})

					Convey("diamond", func() {
						z := s.ensureQuest(c, "z", 1)
						ttest.Drain(c)

						dist.RunTask(c, dm.NewExecutionID(x, 1, 1), func(tsk *fake.Task) error {
							tsk.MustActivate(c, s).DepOn(dm.NewAttemptID(z, 1))
							return nil
						})
						dist.RunTask(c, dm.NewExecutionID(x, 2, 1), func(tsk *fake.Task) error {
							tsk.MustActivate(c, s).DepOn(dm.NewAttemptID(z, 1))
							return nil
						})
						ttest.Drain(c)

						Convey("walk", func() {
							So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
								Quests: map[string]*dm.Quest{
									w: {Attempts: map[uint32]*dm.Attempt{1: {}}},
									x: {Attempts: map[uint32]*dm.Attempt{1: {}, 2: {}}},
									z: {Attempts: map[uint32]*dm.Attempt{1: {}}},
								},
							})
						})

						Convey("walk (dfs)", func() {
							req.Mode.Dfs = true
							So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
								Quests: map[string]*dm.Quest{
									w: {Attempts: map[uint32]*dm.Attempt{1: {}}},
									x: {Attempts: map[uint32]*dm.Attempt{1: {}, 2: {}}},
									z: {Attempts: map[uint32]*dm.Attempt{1: {}}},
								},
							})
						})

					})

					Convey("attemptlist", func() {
						req.Limit.MaxDepth = 1
						req.Include.ObjectIds = true
						req.Query = dm.AttemptListQueryL(map[string][]uint32{x: nil})
						So(req, fake.WalkShouldReturn(c, s), &dm.GraphData{
							Quests: map[string]*dm.Quest{
								x: {
									Id: dm.NewQuestID(x),
									Attempts: map[uint32]*dm.Attempt{
										1: {Id: dm.NewAttemptID(x, 1)},
										2: {Id: dm.NewAttemptID(x, 2)},
									},
								},
							},
						})
					})

				})

				// This is disabled because it was flaky.
				// BUG: crbug.com/621170
				SkipConvey("early stop", func() {
					req.Limit.MaxDepth = 100
					req.Limit.MaxTime = google_pb.NewDuration(time.Nanosecond)
					tc := clock.Get(c).(testclock.TestClock)
					tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
						tc.Add(d + time.Second)
					})
					ret, err := s.WalkGraph(c, req)
					So(err, ShouldBeNil)
					So(ret.HadMore, ShouldBeTrue)
				})

			})
		})
	})
}
