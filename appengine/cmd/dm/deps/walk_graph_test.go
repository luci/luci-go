// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	google_pb "github.com/luci/luci-go/common/proto/google"
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
		ttest := &tumble.Testing{}
		c := ttest.Context()

		ds := datastore.Get(c)
		s := newDecoratedDeps()

		req := &dm.WalkGraphReq{
			Query: dm.AttemptListQueryL(map[string][]uint32{"quest": {1}}),
			Limit: &dm.WalkGraphReq_Limit{MaxDepth: 1},
		}
		So(req.Normalize(), ShouldBeNil)

		Convey("no attempt", func() {
			So(req, WalkShouldReturn(c), &dm.GraphData{
				Quests: map[string]*dm.Quest{"quest": {
					Attempts: map[uint32]*dm.Attempt{
						1: {DNE: true},
					},
				}},
			})
		})

		Convey("good", func() {
			ds.Testable().Consistent(true)

			wDesc := &dm.Quest_Desc{
				DistributorConfigName: "foof",
				JsonPayload:           `{"name":"w"}`,
			}
			w := ensureQuest(c, "w", 1)
			ttest.Drain(c)
			aid := dm.NewAttemptID(w, 1)

			req.Query.AttemptList = dm.NewAttemptList(
				map[string][]uint32{w: {1}})

			Convey("include nothing", func() {
				So(req, WalkShouldReturn(c), &dm.GraphData{
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
				So(req, WalkShouldReturn(c), &dm.GraphData{
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

				So(req, WalkShouldReturn(c), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Data: &dm.Quest_Data{
								Desc: wDesc,
							},
							Attempts: map[uint32]*dm.Attempt{
								1: dm.NewAttemptNeedsExecution(time.Time{}),
							},
						},
					},
				})
			})

			Convey("finished", func() {
				_, err := s.FinishAttempt(c, &dm.FinishAttemptReq{
					Auth:       activate(c, execute(c, aid)),
					JsonResult: `{"data": ["very", "yes"]}`,
					Expiration: google_pb.NewTimestamp(clock.Now(c).Add(time.Hour * 24 * 4)),
				})
				So(err, ShouldBeNil)

				ex := &model.Execution{ID: 1, Attempt: ds.MakeKey("Attempt", aid.DMEncoded())}
				So(ds.Get(ex), ShouldBeNil)
				ex.State = dm.Execution_FINISHED
				So(ds.Put(ex), ShouldBeNil)

				req.Include.AttemptData = true
				req.Include.AttemptResult = true
				req.Include.NumExecutions = 128
				data := `{"data":["very","yes"]}`
				aExpect := dm.NewAttemptFinished(time.Time{}, uint32(len(data)), data)
				aExpect.Data.NumExecutions = 1
				aExpect.Executions = map[uint32]*dm.Execution{1: {
					Data: &dm.Execution_Data{
						State: dm.Execution_FINISHED,
					},
				}}
				So(req, WalkShouldReturn(c), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Attempts: map[uint32]*dm.Attempt{1: aExpect},
						},
					},
				})
			})

			Convey("limited attempt results", func() {
				_, err := s.FinishAttempt(c, &dm.FinishAttemptReq{
					Auth:       activate(c, execute(c, aid)),
					JsonResult: `{"data": ["very", "yes"]}`,
					Expiration: google_pb.NewTimestamp(clock.Now(c).Add(time.Hour * 24 * 4)),
				})
				So(err, ShouldBeNil)

				ex := &model.Execution{ID: 1, Attempt: ds.MakeKey("Attempt", aid.DMEncoded())}
				So(ds.Get(ex), ShouldBeNil)
				ex.State = dm.Execution_FINISHED
				So(ds.Put(ex), ShouldBeNil)

				req.Include.AttemptResult = true
				req.Limit.MaxDataSize = 10
				data := `{"data":["very","yes"]}`
				aExpect := dm.NewAttemptFinished(time.Time{}, uint32(len(data)), "")
				aExpect.Data.NumExecutions = 1
				aExpect.Partial = &dm.Attempt_Partial{Result: dm.Attempt_Partial_DATA_SIZE_LIMIT}
				So(req, WalkShouldReturn(c), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {
							Attempts: map[uint32]*dm.Attempt{1: aExpect},
						},
					},
				})
			})

			Convey("attemptRange", func() {
				x := ensureQuest(c, "x", 1)
				ttest.Drain(c)
				depOn(c, activate(c, execute(c, dm.NewAttemptID(w, 1))),
					dm.NewAttemptID(x, 1), dm.NewAttemptID(x, 2), dm.NewAttemptID(x, 3),
					dm.NewAttemptID(x, 4))
				ttest.Drain(c)

				req.Limit.MaxDepth = 1
				Convey("normal", func() {
					req.Query = dm.AttemptRangeQuery(x, 2, 4)
					So(req, WalkShouldReturn(c), &dm.GraphData{
						Quests: map[string]*dm.Quest{
							x: {Attempts: map[uint32]*dm.Attempt{2: {}, 3: {}}},
						},
					})
				})

				Convey("oob range", func() {
					req.Query = dm.AttemptRangeQuery(x, 2, 6)
					So(req, WalkShouldReturn(c), &dm.GraphData{
						Quests: map[string]*dm.Quest{
							x: {Attempts: map[uint32]*dm.Attempt{
								2: {}, 3: {}, 4: {}, 5: {DNE: true}}},
						},
					})
				})
			})

			Convey("filtered attempt results", func() {
				x := ensureQuest(c, "x", 2)
				ttest.Drain(c)
				depOn(c, activate(c, execute(c, dm.NewAttemptID(w, 1))), dm.NewAttemptID(x, 1))
				ttest.Drain(c)

				exp := google_pb.NewTimestamp(datastore.RoundTime(clock.Now(c).Add(time.Hour * 24 * 4)))

				x1data := `{"data":["I can see this"]}`
				_, err := s.FinishAttempt(c, &dm.FinishAttemptReq{
					Auth:       activate(c, execute(c, dm.NewAttemptID(x, 1))),
					JsonResult: x1data,
					Expiration: exp,
				})
				So(err, ShouldBeNil)

				x2data := `{"data":["nope"]}`
				_, err = s.FinishAttempt(c, &dm.FinishAttemptReq{
					Auth:       activate(c, execute(c, dm.NewAttemptID(x, 2))),
					JsonResult: x2data,
					Expiration: exp,
				})
				So(err, ShouldBeNil)
				ttest.Drain(c)

				req.Auth = activate(c, execute(c, dm.NewAttemptID(w, 1)))
				req.Limit.MaxDepth = 2
				req.Include.AttemptResult = true
				req.Query = dm.AttemptListQueryL(map[string][]uint32{x: nil})

				x1Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x1data)), x1data)
				x1Expect.Data.NumExecutions = 1

				x2Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x2data)), "")
				x2Expect.Partial = &dm.Attempt_Partial{Result: dm.Attempt_Partial_NOT_AUTHORIZED}
				x2Expect.Data.NumExecutions = 1

				So(req, WalkShouldReturn(c), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						x: {Attempts: map[uint32]*dm.Attempt{
							1: x1Expect,
							2: x2Expect,
						}},
					}})
			})

			Convey("own attempt results", func() {
				x := ensureQuest(c, "x", 2)
				ttest.Drain(c)
				depOn(c, activate(c, execute(c, dm.NewAttemptID(w, 1))), dm.NewAttemptID(x, 1))
				ttest.Drain(c)

				exp := google_pb.NewTimestamp(datastore.RoundTime(clock.Now(c).Add(time.Hour * 24 * 4)))

				x1data := `{"data":["I can see this"]}`
				_, err := s.FinishAttempt(c, &dm.FinishAttemptReq{
					Auth:       activate(c, execute(c, dm.NewAttemptID(x, 1))),
					JsonResult: x1data,
					Expiration: exp,
				})
				So(err, ShouldBeNil)
				ttest.Drain(c)

				req.Auth = activate(c, execute(c, dm.NewAttemptID(w, 1)))
				req.Limit.MaxDepth = 2
				req.Include.AttemptResult = true
				req.Query = dm.AttemptListQueryL(map[string][]uint32{w: {1}})

				x1Expect := dm.NewAttemptFinished(time.Time{}, uint32(len(x1data)), x1data)
				x1Expect.Data.NumExecutions = 1

				w1Exepct := dm.NewAttemptExecuting(2)
				w1Exepct.Data.NumExecutions = 2

				// This filter ensures that WalkShouldReturn is using the optimized
				// path for deps traversal when starting from an authed attempt.
				c = datastore.AddRawFilters(c, func(c context.Context, ri datastore.RawInterface) datastore.RawInterface {
					return breakFwdDepLoads{ri}
				})

				So(req, WalkShouldReturn(c), &dm.GraphData{
					Quests: map[string]*dm.Quest{
						w: {Attempts: map[uint32]*dm.Attempt{1: w1Exepct}},
						x: {Attempts: map[uint32]*dm.Attempt{1: x1Expect}},
					}})
			})

			Convey("deps (no dest attempts)", func() {
				req.Limit.MaxDepth = 3
				w1auth := activate(c, execute(c, dm.NewAttemptID(w, 1)))
				x := ensureQuest(c, "x", 1)
				ttest.Drain(c)
				depOn(c, w1auth, dm.NewAttemptID(x, 1), dm.NewAttemptID(x, 2))

				Convey("before tumble", func() {
					Convey("deps", func() {
						req.Include.FwdDeps = true
						// didn't run tumble, so that x|1 and x|2 don't get created.
						So(req, WalkShouldReturn(c), &dm.GraphData{
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
				})

				Convey("after tumble", func() {
					ttest.Drain(c)
					Convey("deps (with dest attempts)", func() {
						req.Include.FwdDeps = true
						req.Include.BackDeps = true
						So(req, WalkShouldReturn(c), &dm.GraphData{
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
						z := ensureQuest(c, "z", 1)
						ttest.Drain(c)
						depOn(c, activate(c, execute(c, dm.NewAttemptID(x, 1))), dm.NewAttemptID(z, 1))
						depOn(c, activate(c, execute(c, dm.NewAttemptID(x, 2))), dm.NewAttemptID(z, 1))
						ttest.Drain(c)

						So(req, WalkShouldReturn(c), &dm.GraphData{
							Quests: map[string]*dm.Quest{
								w: {Attempts: map[uint32]*dm.Attempt{1: {}}},
								x: {Attempts: map[uint32]*dm.Attempt{1: {}, 2: {}}},
								z: {Attempts: map[uint32]*dm.Attempt{1: {}}},
							},
						})
					})

					Convey("diamond (dfs)", func() {
						z := ensureQuest(c, "z", 1)
						ttest.Drain(c)
						depOn(c, activate(c, execute(c, dm.NewAttemptID(x, 1))), dm.NewAttemptID(z, 1))
						depOn(c, activate(c, execute(c, dm.NewAttemptID(x, 2))), dm.NewAttemptID(z, 1))
						ttest.Drain(c)

						req.Mode.Dfs = true
						So(req, WalkShouldReturn(c), &dm.GraphData{
							Quests: map[string]*dm.Quest{
								w: {Attempts: map[uint32]*dm.Attempt{1: {}}},
								x: {Attempts: map[uint32]*dm.Attempt{1: {}, 2: {}}},
								z: {Attempts: map[uint32]*dm.Attempt{1: {}}},
							},
						})
					})

					Convey("attemptlist", func() {
						req.Limit.MaxDepth = 1
						req.Include.ObjectIds = true
						req.Query = dm.AttemptListQueryL(map[string][]uint32{x: nil})
						So(req, WalkShouldReturn(c), &dm.GraphData{
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

				Convey("early stop", func() {
					req.Limit.MaxDepth = 100
					req.Limit.MaxTime = google_pb.NewDuration(time.Nanosecond)
					tc := clock.Get(c).(testclock.TestClock)
					tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
						tc.Add(d + time.Second)
					})
					ret, err := newDecoratedDeps().WalkGraph(c, req)
					So(err, ShouldBeNil)
					So(ret.HadMore, ShouldBeTrue)
				})

			})
		})
	})
}
