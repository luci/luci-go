// Copyright 2022 The LUCI Authors.
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

package tjupdate

import (
	"context"
	"math/rand"
	"testing"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/api/recipe/v1"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// mockRMNotifier is a fake Run Manager Notifier used in Updater for tests.
type mockRMNotifier struct {
	notifiedRuns common.RunIDs
}

func (n *mockRMNotifier) NotifyTryjobsUpdated(ctx context.Context, run common.RunID, _ *tryjob.TryjobUpdatedEvents) error {
	n.notifiedRuns = append(n.notifiedRuns, run)
	return nil
}

type returnValues struct {
	s   tryjob.Status
	r   *tryjob.Result
	err error
}

type mockBackend struct {
	returns []*returnValues
}

func (b *mockBackend) Fetch(ctx context.Context, luciProject string, eid tryjob.ExternalID) (tryjob.Status, *tryjob.Result, error) {
	var ret *returnValues
	switch len(b.returns) {
	case 0:
		return 0, nil, nil
	case 1:
		ret = b.returns[0]
	default:
		ret = b.returns[0]
		b.returns = b.returns[1:]
	}
	return ret.s, ret.r, ret.err
}

func (b *mockBackend) Parse(ctx context.Context, data any) (tryjob.Status, *tryjob.Result, error) {
	ret := data.(*returnValues)
	return ret.s, ret.r, ret.err
}

func (b *mockBackend) Kind() string {
	return "buildbucket"
}

func makeTryjob(ctx context.Context) (*tryjob.Tryjob, error) {
	return makeTryjobWithStatus(ctx, tryjob.Status_TRIGGERED)
}

func makeTestRunID(ctx context.Context, someNumber int64) common.RunID {
	mockDigest := []byte{
		byte(0xff & someNumber),
		byte(0xff & (someNumber >> 8)),
		byte(0xff & (someNumber >> 16)),
		byte(0xff & (someNumber >> 24))}
	return common.MakeRunID("test", clock.Now(ctx), 1, mockDigest)
}

func makeTryjobWithStatus(ctx context.Context, status tryjob.Status) (*tryjob.Tryjob, error) {
	buildID := int64(rand.Int31())
	return makeTryjobWithDetails(ctx, buildID, status, makeTestRunID(ctx, buildID), nil)
}

func makeTryjobWithDetails(ctx context.Context, buildID int64, status tryjob.Status, triggerer common.RunID, reusers common.RunIDs) (*tryjob.Tryjob, error) {
	tj := tryjob.MustBuildbucketID("cr-buildbucket.example.com", buildID).MustCreateIfNotExists(ctx)
	tj.Definition = tryjobDef
	tj.LaunchedBy = triggerer
	tj.ReusedBy = reusers
	tj.Status = status
	return tj, datastore.Put(ctx, tj)
}

var tryjobDef = &tryjob.Definition{
	Backend: &tryjob.Definition_Buildbucket_{
		Buildbucket: &tryjob.Definition_Buildbucket{
			Host: "cr-buildbucket.example.com",
			Builder: &bbpb.BuilderID{
				Project: "test",
				Bucket:  "test_bucket",
				Builder: "test_builder",
			},
		},
	},
	Critical: true,
}

func TestHandleTask(t *testing.T) {
	Convey("HandleTryjobUpdateTask", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		rn := &mockRMNotifier{}
		mb := &mockBackend{}
		updater := NewUpdater(ct.Env, tryjob.NewNotifier(ct.TQDispatcher), rn)
		updater.RegisterBackend(mb)

		Convey("noop", func() {
			tj, err := makeTryjob(ctx)
			So(err, ShouldBeNil)
			eid := tj.ExternalID
			originalEVersion := tj.EVersion
			mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, nil, nil}}

			So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{ExternalId: string(eid)}), ShouldBeNil)
			So(rn.notifiedRuns, ShouldHaveLength, 0)

			// Reload to ensure no changes took place.
			tj = eid.MustCreateIfNotExists(ctx)
			So(tj.EVersion, ShouldEqual, originalEVersion)
		})
		Convey("succeeds updating", func() {
			Convey("status and result", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)
				r := &run.Run{
					ID:            tj.LaunchedBy,
					ConfigGroupID: prjcfg.MakeConfigGroupID("deedbeef", "test_config_group"),
					Tryjobs: &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Requirement: &tryjob.Requirement{
								Definitions: []*tryjob.Definition{tryjobDef},
							},
							Executions: []*tryjob.ExecutionState_Execution{
								{
									Attempts: []*tryjob.ExecutionState_Execution_Attempt{
										{
											TryjobId:   int64(tj.ID),
											ExternalId: string(tj.ExternalID),
											Status:     tj.Status,
											Result:     tj.Result,
										},
									},
								},
							},
						},
					},
				}
				So(datastore.Put(ctx, r), ShouldBeNil)
				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				Convey("by internal ID", func() {
					So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						Id: int64(tj.ID),
					}), ShouldBeNil)
				})
				Convey("by external ID", func() {
					So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						ExternalId: string(tj.ExternalID),
					}), ShouldBeNil)
				})
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				// Ensure status updated.
				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_ENDED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
					So(ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), ShouldEqual, 1)
				})
			})

			Convey("result only", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, &tryjob.Result{Status: tryjob.Result_UNKNOWN}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_UNKNOWN)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("status only", func() {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_PENDING)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, nil, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result, ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("don't emit metrics if tryjob is already in end status", func() {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_ENDED)
				So(err, ShouldBeNil)

				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{
					Status: tryjob.Result_SUCCEEDED,
					Output: &recipe.Output{
						Retry: recipe.Output_OUTPUT_RETRY_DENIED,
					},
				}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("and notifying triggerer and reuser Runs", func() {
				buildID := int64(rand.Int31())
				triggerer := makeTestRunID(ctx, buildID)
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100), makeTestRunID(ctx, buildID+200)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_PENDING, triggerer, reusers)
				So(err, ShouldBeNil)

				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, &tryjob.Result{Status: tryjob.Result_UNKNOWN}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				// Should have called notifier thrice.
				So(rn.notifiedRuns, ShouldHaveLength, 3)
				So(rn.notifiedRuns.Equal(common.RunIDs{triggerer, reusers[0], reusers[1]}), ShouldBeTrue)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})
			Convey("and notifying reuser Run with no triggerer Run", func() {
				buildID := int64(rand.Int31())
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_TRIGGERED, "", reusers)
				So(err, ShouldBeNil)

				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, reusers[0])
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})
		})
		Convey("fails to", func() {
			tj, err := makeTryjob(ctx)
			So(err, ShouldBeNil)

			Convey("update a tryjob with an ID that doesn't exist", func() {
				So(datastore.Delete(ctx, tj), ShouldBeNil)
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id: int64(tj.ID),
				}), ShouldErrLike, "unknown Tryjob with ID")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with an external ID that doesn't exist", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					ExternalId: string(tryjob.MustBuildbucketID("does-not-exist.example.com", 1)),
				}), ShouldErrLike, "unknown Tryjob with ExternalID")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with neither internal nor external ID", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{}), ShouldErrLike, "expected at least one of {Id, ExternalId}")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with mismatching internal and external IDs", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id:         int64(tj.ID),
					ExternalId: string(tryjob.MustBuildbucketID("cr-buildbucket.example.com", 1)),
				}), ShouldErrLike, "the given internal and external IDs for the Tryjob do not match")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Update", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		rn := &mockRMNotifier{}
		mb := &mockBackend{}
		updater := NewUpdater(ct.Env, tryjob.NewNotifier(ct.TQDispatcher), rn)
		updater.RegisterBackend(mb)

		Convey("noop", func() {
			tj, err := makeTryjob(ctx)
			So(err, ShouldBeNil)
			tj.Result = &tryjob.Result{
				UpdateTime: timestamppb.New(clock.Now(ctx)),
			}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			eid := tj.ExternalID
			originalEVersion := tj.EVersion
			var data any
			Convey("if status and result is not changed", func() {
				data = &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						UpdateTime: timestamppb.New(clock.Now(ctx)),
					},
				}
			})
			Convey("if CV has newer data", func() {
				data = &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						UpdateTime: timestamppb.New(clock.Now(ctx).Add(-2 * time.Minute)),
						Output: &recipe.Output{
							Reusability: &recipe.Output_Reusability{
								ModeAllowlist: []string{string(run.DryRun)},
							},
						},
					},
				}
			})

			So(updater.Update(ctx, eid, data), ShouldBeNil)
			So(rn.notifiedRuns, ShouldHaveLength, 0)

			// Reload to ensure no changes took place.
			tj = eid.MustLoad(ctx)
			So(tj.EVersion, ShouldEqual, originalEVersion)
		})
		Convey("succeeds updating", func() {
			Convey("status and result", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)
				r := &run.Run{
					ID:            tj.LaunchedBy,
					ConfigGroupID: prjcfg.MakeConfigGroupID("deedbeef", "test_config_group"),
					Tryjobs: &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Requirement: &tryjob.Requirement{
								Definitions: []*tryjob.Definition{tryjobDef},
							},
							Executions: []*tryjob.ExecutionState_Execution{
								{
									Attempts: []*tryjob.ExecutionState_Execution_Attempt{
										{
											TryjobId:   int64(tj.ID),
											ExternalId: string(tj.ExternalID),
											Status:     tj.Status,
											Result:     tj.Result,
										},
									},
								},
							},
						},
					},
				}
				So(datastore.Put(ctx, r), ShouldBeNil)
				originalEVersion := tj.EVersion
				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_ENDED,
					r: &tryjob.Result{
						Status:     tryjob.Result_SUCCEEDED,
						UpdateTime: timestamppb.New(clock.Now(ctx)),
					},
				}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				// Ensure status updated.
				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_ENDED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
					So(ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), ShouldEqual, 1)
				})
			})

			Convey("result only", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						Status: tryjob.Result_UNKNOWN,
					},
				}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_UNKNOWN)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("status only", func() {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_PENDING)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_TRIGGERED,
				}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result, ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("don't emit metrics if tryjob is already in end status", func() {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_ENDED)
				So(err, ShouldBeNil)

				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_ENDED,
					r: &tryjob.Result{
						Status: tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Retry: recipe.Output_OUTPUT_RETRY_DENIED,
						},
					},
				}), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("and notifying triggerer and reuser Runs", func() {
				buildID := int64(rand.Int31())
				triggerer := makeTestRunID(ctx, buildID)
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100), makeTestRunID(ctx, buildID+200)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_PENDING, triggerer, reusers)
				So(err, ShouldBeNil)

				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						Status: tryjob.Result_UNKNOWN,
					},
				}), ShouldBeNil)
				// Should have called notifier thrice.
				So(rn.notifiedRuns, ShouldHaveLength, 3)
				So(rn.notifiedRuns.Equal(common.RunIDs{triggerer, reusers[0], reusers[1]}), ShouldBeTrue)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})
			Convey("and notifying reuser Run with no triggerer Run", func() {
				buildID := int64(rand.Int31())
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_TRIGGERED, "", reusers)
				So(err, ShouldBeNil)

				So(updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_ENDED,
					r: &tryjob.Result{
						Status: tryjob.Result_SUCCEEDED,
					},
				}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, reusers[0])
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})
		})

		Convey("fails to update a tryjob with an external ID that doesn't exist", func() {
			So(updater.Update(ctx, tryjob.MustBuildbucketID("does-not-exist.example.com", 1), nil), ShouldErrLike, "unknown Tryjob with ExternalID")
			So(rn.notifiedRuns, ShouldHaveLength, 0)
		})
	})
}
