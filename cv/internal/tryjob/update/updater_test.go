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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

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
	tj.Result = &tryjob.Result{
		UpdateTime: timestamppb.New(clock.Now(ctx).UTC()),
	}
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
	ftt.Run("HandleTryjobUpdateTask", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mb := &mockBackend{}
		updater := NewUpdater(ct.Env, tryjob.NewNotifier(ct.TQDispatcher), run.NewNotifier(ct.TQDispatcher))
		updater.RegisterBackend(mb)

		t.Run("noop", func(t *ftt.Test) {
			tj, err := makeTryjob(ctx)
			assert.That(t, err, should.ErrLike(nil))

			eid := tj.ExternalID
			originalEVersion := tj.EVersion

			check := func(t testing.TB) {
				t.Helper()
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{ExternalId: string(eid)}), should.ErrLike(nil), truth.LineContext())
				// Reload to ensure no changes took place.
				tj = eid.MustLoad(ctx)
				assert.That(t, tj.EVersion, should.Equal(originalEVersion), truth.LineContext())
			}
			t.Run("if status and result is not changed", func(t *ftt.Test) {
				mb.returns = []*returnValues{{tj.Status, tj.Result, nil}}
				check(t)
			})

			t.Run("if CV has newer data", func(t *ftt.Test) {
				mb.returns = []*returnValues{{tryjob.Status_PENDING, &tryjob.Result{
					UpdateTime: timestamppb.New(ct.Clock.Now().Add(-2 * time.Minute)),
				}, nil}}
				check(t)
			})
		})
		t.Run("succeeds updating", func(t *ftt.Test) {
			t.Run("status and result", func(t *ftt.Test) {
				tj, err := makeTryjob(ctx)
				assert.That(t, err, should.ErrLike(nil))
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
				assert.That(t, datastore.Put(ctx, r), should.ErrLike(nil))
				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}

				check := func(t testing.TB) {
					t.Helper()
					// Ensure status updated.
					tj = tj.ExternalID.MustCreateIfNotExists(ctx)
					assert.That(t, tj.Status, should.Equal(tryjob.Status_ENDED), truth.LineContext())
					assert.That(t, tj.Result.Status, should.Equal(tryjob.Result_SUCCEEDED), truth.LineContext())
					tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
						assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), should.Equal(1), truth.LineContext())
					})
				}

				t.Run("by internal ID", func(t *ftt.Test) {
					assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						Id: int64(tj.ID),
					}), should.ErrLike(nil))
					check(t)
				})
				t.Run("by external ID", func(t *ftt.Test) {
					assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						ExternalId: string(tj.ExternalID),
					}), should.ErrLike(nil))
					check(t)
				})
			})

			t.Run("result only", func(t *ftt.Test) {
				tj, err := makeTryjob(ctx)
				assert.That(t, err, should.ErrLike(nil))

				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, &tryjob.Result{Status: tryjob.Result_UNKNOWN}, nil}}
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), should.ErrLike(nil))

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				assert.That(t, tj.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.That(t, tj.Result.Status, should.Equal(tryjob.Result_UNKNOWN))
			})

			t.Run("status only", func(t *ftt.Test) {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_PENDING)
				assert.That(t, err, should.ErrLike(nil))

				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, nil, nil}}
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), should.ErrLike(nil))

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				assert.That(t, tj.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.Loosely(t, tj.Result, should.BeNil)
			})

			t.Run("don't emit metrics if tryjob is already in end status", func(t *ftt.Test) {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_ENDED)
				assert.That(t, err, should.ErrLike(nil))

				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{
					Status: tryjob.Result_SUCCEEDED,
					Output: &recipe.Output{
						Retry: recipe.Output_OUTPUT_RETRY_DENIED,
					},
				}, nil}}
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), should.ErrLike(nil))
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
					assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), should.BeNil)
				})
			})
		})
		t.Run("fails to", func(t *ftt.Test) {
			tj, err := makeTryjob(ctx)
			assert.That(t, err, should.ErrLike(nil))

			t.Run("update a tryjob with an ID that doesn't exist", func(t *ftt.Test) {
				assert.That(t, datastore.Delete(ctx, tj), should.ErrLike(nil))
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id: int64(tj.ID),
				}), should.ErrLike("unknown Tryjob with ID"))
			})
			t.Run("update a tryjob with an external ID that doesn't exist", func(t *ftt.Test) {
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					ExternalId: string(tryjob.MustBuildbucketID("does-not-exist.example.com", 1)),
				}), should.ErrLike("unknown Tryjob with ExternalID"))
			})
			t.Run("update a tryjob with neither internal nor external ID", func(t *ftt.Test) {
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{}), should.ErrLike("expected at least one of {Id, ExternalId}"))
			})
			t.Run("update a tryjob with mismatching internal and external IDs", func(t *ftt.Test) {
				assert.That(t, updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id:         int64(tj.ID),
					ExternalId: string(tryjob.MustBuildbucketID("cr-buildbucket.example.com", 1)),
				}), should.ErrLike("the given internal and external IDs for the Tryjob do not match"))
			})
		})
	})
}

func TestUpdate(t *testing.T) {
	ftt.Run("Update", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mb := &mockBackend{}
		updater := NewUpdater(ct.Env, tryjob.NewNotifier(ct.TQDispatcher), run.NewNotifier(ct.TQDispatcher))
		updater.RegisterBackend(mb)

		t.Run("noop", func(t *ftt.Test) {
			tj, err := makeTryjob(ctx)
			assert.That(t, err, should.ErrLike(nil))
			tj.Result = &tryjob.Result{
				UpdateTime: timestamppb.New(clock.Now(ctx)),
			}
			assert.That(t, datastore.Put(ctx, tj), should.ErrLike(nil))
			eid := tj.ExternalID
			originalEVersion := tj.EVersion
			var data any

			check := func(t testing.TB) {
				t.Helper()
				assert.That(t, updater.Update(ctx, eid, data), should.ErrLike(nil), truth.LineContext())
				// Reload to ensure no changes took place.
				tj = eid.MustLoad(ctx)
				assert.That(t, tj.EVersion, should.Equal(originalEVersion), truth.LineContext())
			}

			t.Run("if status and result is not changed", func(t *ftt.Test) {
				data = &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						UpdateTime: timestamppb.New(clock.Now(ctx)),
					},
				}
				check(t)
			})

			t.Run("if CV has newer data", func(t *ftt.Test) {
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
				check(t)
			})
		})
		t.Run("succeeds updating", func(t *ftt.Test) {
			t.Run("status and result", func(t *ftt.Test) {
				tj, err := makeTryjob(ctx)
				assert.That(t, err, should.ErrLike(nil))
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
				assert.That(t, datastore.Put(ctx, r), should.ErrLike(nil))
				assert.That(t, updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_ENDED,
					r: &tryjob.Result{
						Status:     tryjob.Result_SUCCEEDED,
						UpdateTime: timestamppb.New(clock.Now(ctx)),
					},
				}), should.ErrLike(nil))

				// Ensure status updated.
				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				assert.That(t, tj.Status, should.Equal(tryjob.Status_ENDED))
				assert.That(t, tj.Result.Status, should.Equal(tryjob.Result_SUCCEEDED))
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
					assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), should.Equal(1))
				})
			})

			t.Run("result only", func(t *ftt.Test) {
				tj, err := makeTryjob(ctx)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_TRIGGERED,
					r: &tryjob.Result{
						Status: tryjob.Result_UNKNOWN,
					},
				}), should.ErrLike(nil))

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				assert.That(t, tj.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.That(t, tj.Result.Status, should.Equal(tryjob.Result_UNKNOWN))
			})

			t.Run("status only", func(t *ftt.Test) {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_PENDING)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_TRIGGERED,
				}), should.ErrLike(nil))

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				assert.That(t, tj.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.Loosely(t, tj.Result, should.BeNil)
			})

			t.Run("don't emit metrics if tryjob is already in end status", func(t *ftt.Test) {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_ENDED)
				assert.That(t, err, should.ErrLike(nil))

				assert.That(t, updater.Update(ctx, tj.ExternalID, &returnValues{
					s: tryjob.Status_ENDED,
					r: &tryjob.Result{
						Status: tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Retry: recipe.Output_OUTPUT_RETRY_DENIED,
						},
					},
				}), should.ErrLike(nil))
				tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
					assert.That(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobEnded, "test", "test_config_group", true, false, apiv0pb.Tryjob_Result_SUCCEEDED.String()), should.BeNil)
				})
			})

		})

		t.Run("fails to update a tryjob with an external ID that doesn't exist", func(t *ftt.Test) {
			assert.That(t, updater.Update(ctx, tryjob.MustBuildbucketID("does-not-exist.example.com", 1), nil), should.ErrLike("unknown Tryjob with ExternalID"))
		})
	})
}
