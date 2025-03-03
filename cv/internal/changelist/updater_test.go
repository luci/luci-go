// Copyright 2021 The LUCI Authors.
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

package changelist

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	gerrit "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(CL{}))
}

func externalTime(ts time.Time) *UpdateCLTask_Hint {
	return &UpdateCLTask_Hint{ExternalUpdateTime: timestamppb.New(ts)}
}

func TestUpdaterSchedule(t *testing.T) {
	t.Parallel()

	ftt.Run("Correctly generate dedup keys for Updater TQ tasks", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		t.Run("Correctly generate dedup keys for Updater TQ tasks", func(t *ftt.Test) {
			t.Run("Diff CLIDs have diff dedup keys", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", Id: 7}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				task.Id = 8
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Diff ExternalID have diff dedup keys", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj"}
				task.ExternalId = "kind1/foo/23"
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				task.ExternalId = "kind4/foo/56"
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Even if ExternalID and internal ID refer to the same CL, they have diff dedup keys", func(t *ftt.Test) {
				t1 := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				t2 := &UpdateCLTask{LuciProject: "proj", Id: 2}
				k1 := makeTaskDeduplicationKey(ctx, t1, 0)
				k2 := makeTaskDeduplicationKey(ctx, t2, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Diff updatedHint have diff dedup keys", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				task.Hint = externalTime(ct.Clock.Now())
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				task.Hint = externalTime(ct.Clock.Now().Add(time.Second))
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Same CLs but diff LUCI projects have diff dedup keys", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				task.LuciProject += "-diff"
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Same CL at the same time is de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.That(t, k1, should.Match(k2))

				t.Run("Internal ID doesn't affect dedup based on ExternalID", func(t *ftt.Test) {
					task.Id = 123
					k3 := makeTaskDeduplicationKey(ctx, task, 0)
					assert.That(t, k3, should.Match(k1))
				})
			})

			t.Run("Same CL with a delay or after the same delay is de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", Id: 123}
				k1 := makeTaskDeduplicationKey(ctx, task, time.Second)
				ct.Clock.Add(time.Second)
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.That(t, k1, should.Match(k2))
			})

			t.Run("Same CL at mostly same time is also de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				// NOTE: this check may fail if common.DistributeOffset is changed,
				// making new timestamp in the next epoch. If so, adjust the increment.
				ct.Clock.Add(time.Second)
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.That(t, k1, should.Match(k2))
			})

			t.Run("Same CL after sufficient time is no longer de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				k2 := makeTaskDeduplicationKey(ctx, task, blindRefreshInterval)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})

			t.Run("Same CL with the same MetaRevId is de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				task.Hint = &UpdateCLTask_Hint{MetaRevId: "foo"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.That(t, k1, should.Match(k2))
			})

			t.Run("Same CL with the different MetaRevId is not de-duped", func(t *ftt.Test) {
				task := &UpdateCLTask{LuciProject: "proj", ExternalId: "kind1/foo/23"}
				task.Hint = &UpdateCLTask_Hint{MetaRevId: "foo"}
				k1 := makeTaskDeduplicationKey(ctx, task, 0)
				task.Hint = &UpdateCLTask_Hint{MetaRevId: "bar"}
				k2 := makeTaskDeduplicationKey(ctx, task, 0)
				assert.Loosely(t, k1, should.NotResemble(k2))
			})
		})

		t.Run("makeTQTitleForHumans works", func(t *ftt.Test) {
			assert.Loosely(t, makeTQTitleForHumans(&UpdateCLTask{
				LuciProject: "proj",
				Id:          123,
			}), should.Match("proj/123"))
			assert.Loosely(t, makeTQTitleForHumans(&UpdateCLTask{
				LuciProject: "proj",
				ExternalId:  "kind/xyz/44",
				Id:          123,
			}), should.Match("proj/123/kind/xyz/44"))
			assert.Loosely(t, makeTQTitleForHumans(&UpdateCLTask{
				LuciProject: "proj",
				ExternalId:  "gerrit/chromium-review.googlesource.com/1111111",
				Id:          123,
			}), should.Match("proj/123/gerrit/chromium/1111111"))
			assert.Loosely(t, makeTQTitleForHumans(&UpdateCLTask{
				LuciProject: "proj",
				ExternalId:  "gerrit/chromium-review.googlesource.com/1111111",
				Hint:        externalTime(testclock.TestRecentTimeUTC),
			}), should.Match("proj/gerrit/chromium/1111111/u2016-02-03T04:05:06Z"))
		})

		t.Run("Works overall", func(t *ftt.Test) {
			u := NewUpdater(ct.TQDispatcher, nil)
			task := &UpdateCLTask{
				LuciProject: "proj",
				Id:          123,
				Hint:        externalTime(ct.Clock.Now().Add(-time.Second)),
				Requester:   UpdateCLTask_RUN_POKE,
			}
			delay := time.Minute
			assert.Loosely(t, u.ScheduleDelayed(ctx, task, delay), should.BeNil)
			assert.That(t, ct.TQ.Tasks().Payloads(), should.Match([]proto.Message{task}))

			t.Log("Dedup works")
			ct.Clock.Add(delay)
			assert.Loosely(t, u.Schedule(ctx, task), should.BeNil)
			assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.HaveLength(1))

			t.Log("But not within the transaction")
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return u.Schedule(ctx, task)
			}, nil)
			assert.NoErr(t, err)
			assert.That(t, ct.TQ.Tasks().Payloads(), should.Match([]proto.Message{task, task}))

			t.Log("Once out of dedup window, schedules a new task")
			ct.Clock.Add(knownRefreshInterval)
			assert.Loosely(t, u.Schedule(ctx, task), should.BeNil)
			assert.That(t, ct.TQ.Tasks().Payloads(), should.Match([]proto.Message{task, task, task}))
		})
	})
}

func TestUpdaterBatch(t *testing.T) {
	t.Parallel()

	ftt.Run("Correctly handle batches", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		sortedTQPayloads := func() []proto.Message {
			payloads := ct.TQ.Tasks().Payloads()
			sort.Slice(payloads, func(i, j int) bool {
				return payloads[i].(*UpdateCLTask).GetExternalId() < payloads[j].(*UpdateCLTask).GetExternalId()
			})
			return payloads
		}

		u := NewUpdater(ct.TQDispatcher, nil)
		clA := ExternalID("foo/a/1").MustCreateIfNotExists(ctx)
		clB := ExternalID("foo/b/2").MustCreateIfNotExists(ctx)

		expectedPayloads := []proto.Message{
			&UpdateCLTask{
				LuciProject: "proj",
				ExternalId:  "foo/a/1",
				Id:          int64(clA.ID),
				Requester:   UpdateCLTask_RUN_POKE,
			},
			&UpdateCLTask{
				LuciProject: "proj",
				ExternalId:  "foo/b/2",
				Id:          int64(clB.ID),
				Requester:   UpdateCLTask_RUN_POKE,
			},
		}

		t.Run("outside of a transaction, enqueues individual tasks", func(t *ftt.Test) {
			t.Run("special case of just one task", func(t *ftt.Test) {
				err := u.ScheduleBatch(ctx, "proj", []*CL{clA}, UpdateCLTask_RUN_POKE)
				assert.NoErr(t, err)
				assert.That(t, sortedTQPayloads(), should.Match(expectedPayloads[:1]))
			})
			t.Run("multiple", func(t *ftt.Test) {
				err := u.ScheduleBatch(ctx, "proj", []*CL{clA, clB}, UpdateCLTask_RUN_POKE)
				assert.NoErr(t, err)
				assert.That(t, sortedTQPayloads(), should.Match(expectedPayloads))
			})
		})

		t.Run("inside of a transaction, enqueues just one task", func(t *ftt.Test) {
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return u.ScheduleBatch(ctx, "proj", []*CL{clA, clB}, UpdateCLTask_RUN_POKE)
			}, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
			// Run just the batch task.
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(BatchUpdateCLTaskClass))
			assert.That(t, sortedTQPayloads(), should.Match(expectedPayloads))
		})
	})
}

// TestUpdaterWorkingHappyPath is the simplest test for an updater, which also
// illustrates the simplest UpdaterBackend.
func TestUpdaterHappyPath(t *testing.T) {
	t.Parallel()

	ftt.Run("Updater's happy path with simplest possible backend", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pm, rm, tj := pmMock{}, rmMock{}, tjMock{}
		u := NewUpdater(ct.TQDispatcher, NewMutator(ct.TQDispatcher, &pm, &rm, &tj))
		b := &fakeUpdaterBackend{t: t}
		u.RegisterBackend(b)

		////////////////////////////////////////////
		// Phase 1: import CL for the first time. //
		////////////////////////////////////////////

		b.fetchResult = UpdateFields{
			Snapshot: &Snapshot{
				ExternalUpdateTime:    timestamppb.New(ct.Clock.Now().Add(-1 * time.Second)),
				Patchset:              2,
				MinEquivalentPatchset: 1,
				LuciProject:           "luci-project",
				Kind:                  nil, // but should be set in practice,
			},
			ApplicableConfig: &ApplicableConfig{
				Projects: []*ApplicableConfig_Project{
					{
						Name:           "luci-project",
						ConfigGroupIds: []string{"hash/name"},
					},
				},
			},
		}
		// Actually run the Updater.
		assert.Loosely(t, u.handleCL(ctx, &UpdateCLTask{
			LuciProject: "luci-project",
			ExternalId:  "fake/123",
			Requester:   UpdateCLTask_PUBSUB_POLL,
		}), should.BeNil)

		// Ensure that it reported metrics for the CL fetch events.
		assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Internal.CLIngestionAttempted,
			UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
			true,                              // metric:changed == true
			false,                             // metric:dep
			"luci-project",                    // metric:project,
			true,                              // metric:changed_snapshot == true
		), should.Equal(1))
		assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Internal.CLIngestionLatency,
			UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
			false,                             // metric:dep
			"luci-project",                    // metric:project,
			true,                              // metric:changed_snapshot == true
		).Sum(), should.AlmostEqual(1.0))
		assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Internal.CLIngestionLatencyWithoutFetch,
			UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
			false,                             // metric:dep
			"luci-project",                    // metric:project,
			true,                              // metric:changed_snapshot == true
		).Sum(), should.BeGreaterThan(0.0))

		// Ensure CL is created with correct data.
		cl, err := ExternalID("fake/123").Load(ctx)
		assert.NoErr(t, err)
		assert.That(t, cl.Snapshot, should.Match(b.fetchResult.Snapshot))
		assert.That(t, cl.ApplicableConfig, should.Match(b.fetchResult.ApplicableConfig))
		assert.Loosely(t, cl.UpdateTime, should.HappenWithin(time.Microsecond /*see DS.RoundTime()*/, ct.Clock.Now()))

		// Since there are no Runs associated with the CL, the outstanding TQ task
		// should ultimately notify the Project Manager.
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		assert.That(t, pm.byProject, should.Match(map[string]map[common.CLID]int64{
			"luci-project": {cl.ID: cl.EVersion},
		}))

		// Later, a Run will start on this CL.
		const runID = "luci-project/123-1-beef"
		cl.IncompleteRuns = common.RunIDs{runID}
		cl.EVersion++
		assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

		///////////////////////////////////////////////////
		// Phase 2: update the CL with the new patchset. //
		///////////////////////////////////////////////////

		ct.Clock.Add(time.Hour)
		b.reset()
		b.fetchResult.Snapshot = proto.Clone(cl.Snapshot).(*Snapshot)
		b.fetchResult.Snapshot.ExternalUpdateTime = timestamppb.New(ct.Clock.Now())
		b.fetchResult.Snapshot.Patchset++
		b.lookupACfgResult = cl.ApplicableConfig // unchanged

		// Actually run the Updater.
		assert.Loosely(t, u.handleCL(ctx, &UpdateCLTask{
			LuciProject: "luci-project",
			ExternalId:  "fake/123",
		}), should.BeNil)
		cl2 := reloadCL(ctx, cl)

		// The CL entity should have a new patchset and PM/RM should be notified.
		assert.Loosely(t, cl2.Snapshot.GetPatchset(), should.Equal(3))
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		assert.That(t, pm.byProject, should.Match(map[string]map[common.CLID]int64{
			"luci-project": {cl.ID: cl2.EVersion},
		}))
		assert.That(t, rm.byRun, should.Match(map[common.RunID]map[common.CLID]int64{
			runID: {cl.ID: cl2.EVersion},
		}))

		///////////////////////////////////////////////////
		// Phase 3: update if backend detect a change    //
		///////////////////////////////////////////////////
		b.reset()
		b.fetchResult.Snapshot = proto.Clone(cl.Snapshot).(*Snapshot) // unchanged
		b.lookupACfgResult = cl.ApplicableConfig                      // unchanged
		b.backendSnapshotUpdated = true

		// Actually run the Updater.
		assert.Loosely(t, u.handleCL(ctx, &UpdateCLTask{
			LuciProject: "luci-project",
			ExternalId:  "fake/123",
		}), should.BeNil)
		cl3 := reloadCL(ctx, cl)

		// The CL entity have been updated
		assert.Loosely(t, cl3.EVersion, should.BeGreaterThan(cl2.EVersion))
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		assert.That(t, pm.byProject, should.Match(map[string]map[common.CLID]int64{
			"luci-project": {cl.ID: cl3.EVersion},
		}))
		assert.That(t, rm.byRun, should.Match(map[common.RunID]map[common.CLID]int64{
			runID: {cl.ID: cl3.EVersion},
		}))
	})
}

func TestUpdaterFetchedNoNewData(t *testing.T) {
	t.Parallel()

	ftt.Run("Updater skips updating the CL when no new data is fetched", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pm, rm, tj := pmMock{}, rmMock{}, tjMock{}
		u := NewUpdater(ct.TQDispatcher, NewMutator(ct.TQDispatcher, &pm, &rm, &tj))
		b := &fakeUpdaterBackend{t: t}
		u.RegisterBackend(b)

		snap := &Snapshot{
			ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
			Patchset:              2,
			MinEquivalentPatchset: 1,
			LuciProject:           "luci-project",
			Kind:                  nil, // but should be set in practice
		}
		acfg := &ApplicableConfig{Projects: []*ApplicableConfig_Project{
			{
				Name:           "luci-projectj",
				ConfigGroupIds: []string{"hash/old"},
			},
		}}
		// Put an existing CL.
		cl := ExternalID("fake/1").MustCreateIfNotExists(ctx)
		cl.ApplicableConfig = acfg
		cl.Snapshot = snap
		cl.EVersion++
		assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

		t.Run("updaterBackend is aware that there is no new data", func(t *ftt.Test) {
			b.fetchResult = UpdateFields{}
		})
		t.Run("updaterBackend is not aware that it fetched the exact same data", func(t *ftt.Test) {
			b.fetchResult = UpdateFields{
				Snapshot:         snap,
				ApplicableConfig: acfg,
			}
		})

		err := u.handleCL(ctx, &UpdateCLTask{
			LuciProject: "luci-project",
			ExternalId:  "fake/1",
			Requester:   UpdateCLTask_PUBSUB_POLL})

		assert.NoErr(t, err)

		// Check the monitoring data
		if b.fetchResult.IsEmpty() {
			// Empty fetched result implies that it didn't even perform
			// a fetch. Hence, no metrics should have been reported.
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeNil)
		} else {
			// This is the case where a fetch was performed but
			// the data was actually the same as the existing snapshot.
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Internal.CLIngestionAttempted,
				UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
				false,                             // metric:changed == false
				false,                             // metric:dep
				"luci-project",                    // metric:project,
				false,                             // metric:changed_snapshot == false
			), should.Equal(1))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Internal.CLIngestionLatency,
				UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
				false,                             // metric:dep
				"luci-project",                    // metric:project,
				false,                             // metric:changed_snapshot == false,
			), should.BeNil)
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Internal.CLIngestionLatencyWithoutFetch,
				UpdateCLTask_PUBSUB_POLL.String(), // metric:requester,
				false,                             // metric:dep
				"luci-project",                    // metric:project,
				false,                             // metric:changed_snapshot == false
			), should.BeNil)
		}

		// CL entity shouldn't change and notifications should not be emitted.
		cl2 := reloadCL(ctx, cl)
		assert.Loosely(t, cl2.EVersion, should.Equal(cl.EVersion))
		// CL Mutator guarantees that EVersion is bumped on every write, but check
		// the entire CL contents anyway in case there is a buggy by-pass of
		// Mutator somewhere.
		assert.That(t, cl2, should.Match(cl))
		assert.Loosely(t, ct.TQ.Tasks(), should.BeEmpty)
	})
}

func TestUpdaterAccessRestriction(t *testing.T) {
	t.Parallel()

	ftt.Run("Updater works correctly when backend denies access to the CL", t, func(t *ftt.Test) {
		// This is a long test, don't debug it first if other TestUpdater* tests are
		// also failing.
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pm, rm, tj := pmMock{}, rmMock{}, tjMock{}
		u := NewUpdater(ct.TQDispatcher, NewMutator(ct.TQDispatcher, &pm, &rm, &tj))
		b := &fakeUpdaterBackend{t: t}
		u.RegisterBackend(b)

		//////////////////////////////////////////////////////////////////////////
		// Phase 1: prepare an old CL previously fetched for another LUCI project.
		//////////////////////////////////////////////////////////////////////////

		longTimeAgo := ct.Clock.Now().Add(-180 * 24 * time.Hour)
		cl := ExternalID("fake/1").MustCreateIfNotExists(ctx)
		cl.Snapshot = &Snapshot{
			ExternalUpdateTime:    timestamppb.New(longTimeAgo),
			Patchset:              2,
			MinEquivalentPatchset: 1,
			LuciProject:           "previously-existing-project",
			Kind:                  nil, // but should be set in practice,
		}
		cl.ApplicableConfig = &ApplicableConfig{Projects: []*ApplicableConfig_Project{
			{
				Name:           "previously-existing-project",
				ConfigGroupIds: []string{"old-hash/old-name"},
			},
		}}
		alsoLongTimeAgo := longTimeAgo.Add(time.Minute)
		cl.Access = &Access{ByProject: map[string]*Access_Project{
			// One possibility is user makes a typo in the free-from Cq-Depend
			// footer and accidentally referenced a CL from totally different
			// project.
			"another-project-with-invalid-cl-deps": {NoAccessTime: timestamppb.New(alsoLongTimeAgo)},
		}}
		cl.EVersion++
		assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

		//////////////////////////////////////////////////////////////////////////
		// Phase 2: simulate a Fetch which got access denied from backend.
		//////////////////////////////////////////////////////////////////////////

		b.fetchResult = UpdateFields{
			ApplicableConfig: &ApplicableConfig{Projects: []*ApplicableConfig_Project{
				// Note that the old project is no longer watching this CL.
				{
					Name:           "luci-project",
					ConfigGroupIds: []string{"hash/name"},
				},
			}},
			Snapshot: nil, // nothing was actually fetched.
			AddDependentMeta: &Access{ByProject: map[string]*Access_Project{
				"luci-project": {NoAccessTime: timestamppb.New(ct.Clock.Now())},
			}},
		}

		err := u.handleCL(ctx, &UpdateCLTask{LuciProject: "luci-project", ExternalId: "fake/1"})
		assert.NoErr(t, err)

		// Resulting CL entity should keep the Snapshot, rewrite ApplicableConfig,
		// and merge Access.
		cl2 := reloadCL(ctx, cl)
		assert.That(t, cl2.Snapshot, should.Match(cl.Snapshot))
		assert.That(t, cl2.ApplicableConfig, should.Match(b.fetchResult.ApplicableConfig))
		assert.That(t, cl2.Access, should.Match(&Access{ByProject: map[string]*Access_Project{
			"another-project-with-invalid-cl-deps": {NoAccessTime: timestamppb.New(alsoLongTimeAgo)},
			"luci-project":                         {NoAccessTime: timestamppb.New(ct.Clock.Now())},
		}}))
		// Notifications doesn't have to be sent to the project with invalid deps,
		// as this update is irrelevant to the project.
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		assert.That(t, pm.byProject, should.Match(map[string]map[common.CLID]int64{
			"luci-project":                {cl.ID: cl2.EVersion},
			"previously-existing-project": {cl.ID: cl2.EVersion},
		}))

		//////////////////////////////////////////////////////////////////////////
		// Phase 3: backend ACLs are fixed.
		//////////////////////////////////////////////////////////////////////////
		ct.Clock.Add(time.Hour)
		b.reset()
		b.fetchResult = UpdateFields{
			Snapshot: &Snapshot{
				ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
				Patchset:              4,
				MinEquivalentPatchset: 1,
				LuciProject:           "luci-project",
				Kind:                  nil, // but should be set in practice
			},
			ApplicableConfig: cl2.ApplicableConfig, // same as before
			DelAccess:        []string{"luci-project"},
		}
		err = u.handleCL(ctx, &UpdateCLTask{LuciProject: "luci-project", ExternalId: "fake/1"})
		assert.NoErr(t, err)
		cl3 := reloadCL(ctx, cl)
		assert.That(t, cl3.Snapshot, should.Match(b.fetchResult.Snapshot))                     // replaced
		assert.That(t, cl3.ApplicableConfig, should.Match(cl2.ApplicableConfig))               // same
		assert.That(t, cl3.Access, should.Match(&Access{ByProject: map[string]*Access_Project{ // updated
			// No more "luci-project" entry.
			"another-project-with-invalid-cl-deps": {NoAccessTime: timestamppb.New(alsoLongTimeAgo)},
		}}))
		// Notifications are still not sent to the project with invalid deps.
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		assert.That(t, pm.byProject, should.Match(map[string]map[common.CLID]int64{
			"luci-project":                {cl.ID: cl3.EVersion},
			"previously-existing-project": {cl.ID: cl3.EVersion},
		}))
	})
}

func TestUpdaterHandlesErrors(t *testing.T) {
	t.Parallel()

	ftt.Run("Updater handles errors", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		u := NewUpdater(ct.TQDispatcher, nil)

		t.Run("bails permanently in cases which should not happen", func(t *ftt.Test) {
			t.Run("No ID given", func(t *ftt.Test) {
				err := u.handleCL(ctx, &UpdateCLTask{
					LuciProject: "luci-project",
				})
				assert.Loosely(t, err, should.ErrLike("invalid task input"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
			t.Run("No LUCI project given", func(t *ftt.Test) {
				err := u.handleCL(ctx, &UpdateCLTask{
					ExternalId: "fake/1",
				})
				assert.Loosely(t, err, should.ErrLike("invalid task input"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
			t.Run("Contradicting external and internal IDs", func(t *ftt.Test) {
				cl1 := ExternalID("fake/1").MustCreateIfNotExists(ctx)
				cl2 := ExternalID("fake/2").MustCreateIfNotExists(ctx)
				err := u.handleCL(ctx, &UpdateCLTask{
					LuciProject: "luci-project",
					Id:          int64(cl1.ID),
					ExternalId:  string(cl2.ExternalID),
				})
				assert.Loosely(t, err, should.ErrLike("invalid task"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
			t.Run("Internal ID doesn't actually exist", func(t *ftt.Test) {
				// While in most cases this is a bug, it can happen in prod
				// if an old CL is being deleted due to data retention policy at the
				// same time as something else inside the CV is requesting a refresh of
				// the CL against external system. One example of this is if a new CL
				// mistakenly marked a very old CL as a dependency.
				err := u.handleCL(ctx, &UpdateCLTask{
					Id:          404,
					LuciProject: "luci-project",
				})
				assert.Loosely(t, err, should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
			t.Run("CL from unregistered backend", func(t *ftt.Test) {
				err := u.handleCL(ctx, &UpdateCLTask{
					ExternalId:  "unknown/404",
					LuciProject: "luci-project",
				})
				assert.Loosely(t, err, should.ErrLike("backend is not supported"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
		})

		t.Run("Respects TQErrorSpec", func(t *ftt.Test) {
			ignoreMe := errors.New("ignore-me")
			b := &fakeUpdaterBackend{
				t: t,
				tqErrorSpec: common.TQIfy{
					KnownIgnore: []error{ignoreMe},
				},
				fetchError: errors.Annotate(ignoreMe, "something went wrong").Err(),
			}
			u.RegisterBackend(b)
			err := u.handleCL(ctx, &UpdateCLTask{LuciProject: "lp", ExternalId: "fake/1"})
			assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
			assert.Loosely(t, err, should.ErrLike("ignore-me"))
		})
	})
}

func TestUpdaterAvoidsFetchWhenPossible(t *testing.T) {
	t.Parallel()

	ftt.Run("Updater skips fetching when possible", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		u := NewUpdater(ct.TQDispatcher, NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}, &tjMock{}))
		b := &fakeUpdaterBackend{t: t}
		u.RegisterBackend(b)

		// Simulate a perfect case for avoiding the snapshot.
		cl := ExternalID("fake/123").MustCreateIfNotExists(ctx)
		cl.Snapshot = &Snapshot{
			ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
			Patchset:              2,
			MinEquivalentPatchset: 1,
			LuciProject:           "luci-project",
			Kind:                  nil, // but should be set in practice,
		}
		cl.ApplicableConfig = &ApplicableConfig{
			Projects: []*ApplicableConfig_Project{
				{
					Name:           "luci-project",
					ConfigGroupIds: []string{"hash/name"},
				},
			},
		}
		cl.EVersion++
		assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

		task := &UpdateCLTask{
			LuciProject: "luci-project",
			ExternalId:  string(cl.ExternalID),
			Hint:        externalTime(ct.Clock.Now()),
		}
		// Typically, ApplicableConfig config (i.e. which LUCI project watch this
		// CL) doesn't change, too.
		b.lookupACfgResult = cl.ApplicableConfig

		t.Run("skips Fetch", func(t *ftt.Test) {
			t.Run("happy path: everything is up to date", func(t *ftt.Test) {
				assert.Loosely(t, u.handleCL(ctx, task), should.BeNil)

				cl2 := reloadCL(ctx, cl)
				// Quick-fail if EVersion changes.
				assert.Loosely(t, cl2.EVersion, should.Equal(cl.EVersion))
				// Ensure nothing about the CL actually changed.
				assert.That(t, cl2, should.Match(cl))

				assert.Loosely(t, b.wasLookupApplicableConfigCalled(), should.BeTrue)
				assert.Loosely(t, b.wasFetchCalled(), should.BeFalse)
			})

			t.Run("special path: changed ApplicableConfig is saved", func(t *ftt.Test) {
				t.Run("CL is not watched by any project", func(t *ftt.Test) {
					b.lookupACfgResult = &ApplicableConfig{}
				})
				t.Run("CL is watched by another project", func(t *ftt.Test) {
					b.lookupACfgResult = &ApplicableConfig{Projects: []*ApplicableConfig_Project{
						{
							Name:           "other-project",
							ConfigGroupIds: []string{"ohter-hash/other-name"},
						},
					}}
				})
				t.Run("CL is additionally watched by another project", func(t *ftt.Test) {
					b.lookupACfgResult.Projects = append(b.lookupACfgResult.Projects, &ApplicableConfig_Project{
						Name:           "other-project",
						ConfigGroupIds: []string{"ohter-hash/other-name"},
					})
				})
				// Either way, fetch can be skipped & Snapshot can be preserved, but the
				// ApplicableConfig must be updated.
				assert.Loosely(t, u.handleCL(ctx, task), should.BeNil)
				assert.Loosely(t, b.wasFetchCalled(), should.BeFalse)
				cl2 := reloadCL(ctx, cl)
				assert.That(t, cl2.Snapshot, should.Match(cl.Snapshot))
				assert.That(t, cl2.ApplicableConfig, should.Match(b.lookupACfgResult))
			})

			t.Run("meta_rev_id is the same", func(t *ftt.Test) {
				t.Run("even if the CL entity is really old", func(t *ftt.Test) {
					ct.Clock.Add(autoRefreshAfter + time.Minute)
				})

				cl.Snapshot.Kind = &Snapshot_Gerrit{Gerrit: &Gerrit{
					Info: &gerrit.ChangeInfo{MetaRevId: "deadbeef"},
				}}
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
				task.Hint.MetaRevId = "deadbeef"
				assert.Loosely(t, u.handleCL(ctx, task), should.BeNil)
				assert.Loosely(t, b.wasFetchCalled(), should.BeFalse)
			})
		})

		t.Run("doesn't skip Fetch because ...", func(t *ftt.Test) {
			saveCLAndRun := func() {
				cl.EVersion++
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
				assert.Loosely(t, u.handleCL(ctx, task), should.BeNil)
			}
			t.Run("no snapshot", func(t *ftt.Test) {
				cl.Snapshot = nil
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("snapshot marked outdated", func(t *ftt.Test) {
				cl.Snapshot.Outdated = &Snapshot_Outdated{}
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("snapshot is definitely old", func(t *ftt.Test) {
				cl.Snapshot.ExternalUpdateTime.Seconds -= 3600
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("snapshot might be old", func(t *ftt.Test) {
				task.Hint = nil
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("CL entity is really old", func(t *ftt.Test) {
				ct.Clock.Add(autoRefreshAfter + time.Minute)
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("snapshot is for a different project", func(t *ftt.Test) {
				cl.Snapshot.LuciProject = "other"
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("backend isn't sure about applicable config", func(t *ftt.Test) {
				b.lookupACfgResult = nil
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("CL entity has record of prior access restriction", func(t *ftt.Test) {
				cl.Access = &Access{
					ByProject: map[string]*Access_Project{
						"luci-project": {
							// In practice, actual fields are set, but they aren't important
							// for this test.
						},
					},
				}
				saveCLAndRun()
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
			t.Run("meta_rev_id is different", func(t *ftt.Test) {
				cl.Snapshot.Kind = &Snapshot_Gerrit{Gerrit: &Gerrit{
					Info: &gerrit.ChangeInfo{MetaRevId: "deadbeef"},
				}}
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
				task.Hint.MetaRevId = "foo"
				assert.Loosely(t, u.handleCL(ctx, task), should.BeNil)
				assert.Loosely(t, b.wasFetchCalled(), should.BeTrue)
			})
		})

		t.Run("aborts before the Fetch because LookupApplicableConfig failed", func(t *ftt.Test) {
			b.lookupACfgError = errors.New("boo", transient.Tag)
			err := u.handleCL(ctx, task)
			assert.Loosely(t, err, should.ErrLike(b.lookupACfgError))
			assert.Loosely(t, b.wasFetchCalled(), should.BeFalse)
		})
	})
}

func TestUpdaterResolveAndScheduleDepsUpdate(t *testing.T) {
	t.Parallel()

	ftt.Run("ResolveAndScheduleDepsUpdate correctly resolves deps", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		u := NewUpdater(ct.TQDispatcher, NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}, &tjMock{}))

		scheduledUpdates := func() (out []string) {
			for _, p := range ct.TQ.Tasks().Payloads() {
				if task, ok := p.(*UpdateCLTask); ok {
					// Each scheduled task should have ID set, as it is known,
					// to save on future lookup.
					assert.Loosely(t, task.GetId(), should.NotEqual(0))
					e := task.GetExternalId()
					// But also ExternalID, primarily for debugging.
					assert.Loosely(t, e, should.NotBeEmpty)
					out = append(out, e)
				}
			}
			sort.Strings(out)
			return out
		}
		eids := func(cls ...*CL) []string {
			out := make([]string, len(cls))
			for i, cl := range cls {
				out[i] = string(cl.ExternalID)
			}
			sort.Strings(out)
			return out
		}

		// Setup 4 existing CLs in various states.
		const lProject = "luci-project"
		// Various backend IDs are used here for test readability and debug-ability
		// only. In practice, all deps likely come from the same backend.
		var (
			clBareBones           = ExternalID("bare-bones/10").MustCreateIfNotExists(ctx)
			clUpToDate            = ExternalID("up-to-date/12").MustCreateIfNotExists(ctx)
			clUpToDateDiffProject = ExternalID("up-to-date-diff-project/13").MustCreateIfNotExists(ctx)
		)
		clUpToDate.Snapshot = &Snapshot{
			ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
			Patchset:              1,
			MinEquivalentPatchset: 1,
			LuciProject:           lProject,
		}
		clUpToDateDiffProject.Snapshot = proto.Clone(clUpToDate.Snapshot).(*Snapshot)
		clUpToDateDiffProject.Snapshot.LuciProject = "other-project"
		assert.Loosely(t, datastore.Put(ctx, clUpToDate, clUpToDateDiffProject), should.BeNil)

		t.Run("no deps", func(t *ftt.Test) {
			deps, err := u.ResolveAndScheduleDepsUpdate(ctx, lProject, nil, UpdateCLTask_RUN_POKE)
			assert.NoErr(t, err)
			assert.Loosely(t, deps, should.BeEmpty)
		})

		t.Run("only existing CLs", func(t *ftt.Test) {
			deps, err := u.ResolveAndScheduleDepsUpdate(ctx, lProject, map[ExternalID]DepKind{
				clBareBones.ExternalID:           DepKind_SOFT,
				clUpToDate.ExternalID:            DepKind_HARD,
				clUpToDateDiffProject.ExternalID: DepKind_SOFT,
			}, UpdateCLTask_RUN_POKE)
			assert.NoErr(t, err)
			assert.That(t, deps, should.Match(sortDeps([]*Dep{
				{Clid: int64(clBareBones.ID), Kind: DepKind_SOFT},
				{Clid: int64(clUpToDate.ID), Kind: DepKind_HARD},
				{Clid: int64(clUpToDateDiffProject.ID), Kind: DepKind_SOFT},
			})))
			// Update for the `clUpToDate` is not necessary.
			assert.That(t, scheduledUpdates(), should.Match(eids(clBareBones, clUpToDateDiffProject)))
		})

		t.Run("only new CLs", func(t *ftt.Test) {
			deps, err := u.ResolveAndScheduleDepsUpdate(ctx, lProject, map[ExternalID]DepKind{
				"new/1": DepKind_SOFT,
				"new/2": DepKind_HARD,
			}, UpdateCLTask_RUN_POKE)
			assert.NoErr(t, err)
			cl1 := ExternalID("new/1").MustCreateIfNotExists(ctx)
			cl2 := ExternalID("new/2").MustCreateIfNotExists(ctx)
			assert.That(t, deps, should.Match(sortDeps([]*Dep{
				{Clid: int64(cl1.ID), Kind: DepKind_SOFT},
				{Clid: int64(cl2.ID), Kind: DepKind_HARD},
			})))
			assert.That(t, scheduledUpdates(), should.Match(eids(cl1, cl2)))
		})

		t.Run("mix old and new CLs", func(t *ftt.Test) {
			deps, err := u.ResolveAndScheduleDepsUpdate(ctx, lProject, map[ExternalID]DepKind{
				"new/1":                DepKind_SOFT,
				"new/2":                DepKind_HARD,
				clBareBones.ExternalID: DepKind_HARD,
				clUpToDate.ExternalID:  DepKind_SOFT,
			}, UpdateCLTask_RUN_POKE)
			assert.NoErr(t, err)
			cl1 := ExternalID("new/1").MustCreateIfNotExists(ctx)
			cl2 := ExternalID("new/2").MustCreateIfNotExists(ctx)
			assert.That(t, deps, should.Match(sortDeps([]*Dep{
				{Clid: int64(cl1.ID), Kind: DepKind_SOFT},
				{Clid: int64(cl2.ID), Kind: DepKind_HARD},
				{Clid: int64(clBareBones.ID), Kind: DepKind_HARD},
				{Clid: int64(clUpToDate.ID), Kind: DepKind_SOFT},
			})))
			// Update for the `clUpToDate` is not necessary.
			assert.That(t, scheduledUpdates(), should.Match(eids(cl1, cl2, clBareBones)))
		})

		t.Run("high number of dependency CLs", func(t *ftt.Test) {
			const clCount = 1024
			depCLMap := make(map[ExternalID]DepKind, clCount)
			depCLs := make([]*CL, clCount)
			for i := 0; i < clCount; i++ {
				depCLs[i] = ExternalID(fmt.Sprintf("high-dep-cl/%04d", i)).MustCreateIfNotExists(ctx)
				depCLMap[depCLs[i].ExternalID] = DepKind_HARD
			}

			deps, err := u.ResolveAndScheduleDepsUpdate(ctx, lProject, depCLMap, UpdateCLTask_RUN_POKE)
			assert.NoErr(t, err)
			expectedDeps := make([]*Dep, clCount)
			for i, depCL := range depCLs {
				expectedDeps[i] = &Dep{
					Clid: int64(depCL.ID),
					Kind: DepKind_HARD,
				}
			}
			assert.That(t, deps, should.Match(expectedDeps))
			assert.That(t, scheduledUpdates(), should.Match(eids(depCLs...)))
		})
	})
}

// fakeUpdaterBackend is a fake UpdaterBackend.
//
// It provides functionality which is a subset of what gomock would generate,
// but with additional assertions to validate the contract between Updater and
// its backend.
type fakeUpdaterBackend struct {
	t testing.TB

	tqErrorSpec common.TQIfy

	// LookupApplicableConfig related fields:

	lookupACfgCL     *CL // copy
	lookupACfgResult *ApplicableConfig
	lookupACfgError  error

	// Fetch related fields:
	fetchCL          *CL // copy
	fetchProject     string
	fetchUpdatedHint time.Time
	fetchResult      UpdateFields
	fetchError       error

	backendSnapshotUpdated bool
}

func (f *fakeUpdaterBackend) Kind() string {
	return "fake"
}

func (f *fakeUpdaterBackend) TQErrorSpec() common.TQIfy {
	return f.tqErrorSpec
}

func (f *fakeUpdaterBackend) wasLookupApplicableConfigCalled() bool {
	return f.lookupACfgCL != nil
}

func (f *fakeUpdaterBackend) LookupApplicableConfig(ctx context.Context, saved *CL) (*ApplicableConfig, error) {
	f.t.Helper()
	assert.Loosely(f.t, f.wasLookupApplicableConfigCalled(), should.BeFalse, truth.LineContext())

	// Check contract with a backend:
	assert.Loosely(f.t, saved, should.NotBeNil, truth.LineContext())
	assert.Loosely(f.t, saved.ID, should.NotEqual(0), truth.LineContext())
	assert.Loosely(f.t, saved.ExternalID, should.NotBeEmpty, truth.LineContext())
	assert.Loosely(f.t, saved.Snapshot, should.NotBeNil, truth.LineContext())

	// Shallow-copy to catch some mistakes in test.
	f.lookupACfgCL = &CL{}
	*f.lookupACfgCL = *saved
	return f.lookupACfgResult, f.lookupACfgError
}

func (f *fakeUpdaterBackend) wasFetchCalled() bool {
	return f.fetchCL != nil
}

func (f *fakeUpdaterBackend) Fetch(ctx context.Context, in *FetchInput) (UpdateFields, error) {
	assert.Loosely(f.t, f.wasFetchCalled(), should.BeFalse, truth.LineContext())

	// Check contract with a backend:
	assert.Loosely(f.t, in.CL, should.NotBeNil, truth.LineContext())
	assert.Loosely(f.t, in.CL.ExternalID, should.NotBeEmpty, truth.LineContext())

	// Shallow-copy to catch some mistakes in test.
	f.fetchCL = &CL{}
	*f.fetchCL = *in.CL
	f.fetchProject = in.Project
	f.fetchUpdatedHint = in.UpdatedHint
	return f.fetchResult, f.fetchError
}

func (f *fakeUpdaterBackend) HasChanged(cvCurrent, backendCurrent *Snapshot) bool {
	switch {
	case backendCurrent == nil:
		panic("impossible. Backend must have a non-nil snapshot")
	case cvCurrent == nil:
		return true
	case backendCurrent.GetExternalUpdateTime().AsTime().After(cvCurrent.GetExternalUpdateTime().AsTime()):
		return true
	case backendCurrent.GetPatchset() > cvCurrent.GetPatchset():
		return true
	case f.backendSnapshotUpdated:
		return true
	default:
		return false
	}
}

// reset resets the fake for the next use.
func (f *fakeUpdaterBackend) reset() {
	*f = fakeUpdaterBackend{}
}

// reloadCL loads a new CL from Datastore.
//
// Doesn't re-use the object.
func reloadCL(ctx context.Context, cl *CL) *CL {
	ret := &CL{ID: cl.ID}
	if err := datastore.Get(ctx, ret); err != nil {
		panic(err)
	}
	return ret
}
