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
	"math/rand"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestMutatorSingleCL(t *testing.T) {
	t.Parallel()

	ftt.Run("Mutator works on a single CL", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const run1 = lProject + "/1"
		const run2 = lProject + "/2"
		const gHost = "x-review.example.com"
		const gChange = 44
		eid := MustGobID(gHost, gChange)

		pm := pmMock{}
		rm := rmMock{}
		tj := tjMock{}
		m := NewMutator(ct.TQDispatcher, &pm, &rm, &tj)

		execBatchOnCLUpdatedTask := func() {
			assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
			assert.Loosely(t, ct.TQ.Tasks()[0].Class, should.Resemble(BatchOnCLUpdatedTaskClass))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(BatchOnCLUpdatedTaskClass))
		}
		expectNoNotifications := func() {
			assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(0))
			assert.Loosely(t, pm.byProject, should.BeEmpty)
			assert.Loosely(t, rm.byRun, should.BeEmpty)
			assert.Loosely(t, tj.clsNotified, should.BeEmpty)
		}

		t.Run("Upsert method", func(t *ftt.Test) {
			t.Run("creates", func(t *ftt.Test) {
				s := makeSnapshot(lProject, ct.Clock.Now())
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					cl.Snapshot = s
					return nil
				})
				assert.NoErr(t, err)
				assert.That(t, cl.ExternalID, should.Equal(eid))
				assert.Loosely(t, cl.EVersion, should.Equal(1))
				assert.Loosely(t, cl.UpdateTime, should.Match(ct.Clock.Now()))
				assert.Loosely(t, cl.RetentionKey, should.NotBeEmpty)
				assert.Loosely(t, cl.Snapshot, should.Resemble(s))

				execBatchOnCLUpdatedTask()
				assert.Loosely(t, pm.byProject, should.Resemble(map[string]map[common.CLID]int64{
					lProject: {cl.ID: cl.EVersion},
				}))
				assert.Loosely(t, rm.byRun, should.BeEmpty)
				assert.Loosely(t, tj.clsNotified, should.BeEmpty)
			})

			t.Run("skips creation", func(t *ftt.Test) {
				// This is a special case which isn't supposed to be needed,
				// but it's kept here for completeness.
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return ErrStopMutation
				})
				assert.NoErr(t, err)
				assert.Loosely(t, cl, should.BeNil)
				expectNoNotifications()
			})

			t.Run("updates", func(t *ftt.Test) {
				s1 := makeSnapshot(lProject, ct.Clock.Now())
				cl := eid.MustCreateIfNotExists(ctx)
				cl.Snapshot = s1
				cl.IncompleteRuns = common.MakeRunIDs(run1)
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
				s2.MinEquivalentPatchset++
				var priorSnapshot *Snapshot
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					if priorSnapshot == nil {
						priorSnapshot = cl.Snapshot
					}
					cl.Snapshot = s2
					cl.IncompleteRuns.InsertSorted(run2) // idempotent
					return nil
				})
				assert.NoErr(t, err)

				assert.Loosely(t, priorSnapshot, should.Resemble(s1))
				assert.That(t, cl.ExternalID, should.Equal(eid))
				assert.Loosely(t, cl.EVersion, should.Equal(2))
				assert.That(t, cl.UpdateTime, should.Match(ct.Clock.Now().UTC()))
				assert.Loosely(t, cl.RetentionKey, should.NotBeEmpty)
				assert.Loosely(t, cl.Snapshot, should.Resemble(s2))
				assert.Loosely(t, tj.clsNotified, should.HaveLength(1))
				for clid, content := range tj.clsNotified {
					assert.That(t, clid, should.Equal(cl.ID))
					assert.That(t, content.prevMinEquiPS, should.Equal(s1.GetMinEquivalentPatchset()))
					assert.That(t, content.curMinEquiPS, should.Equal(s2.GetMinEquivalentPatchset()))
				}
				execBatchOnCLUpdatedTask()
				assert.Loosely(t, pm.byProject, should.Resemble(map[string]map[common.CLID]int64{
					lProject: {cl.ID: cl.EVersion},
				}))
				assert.Loosely(t, rm.byRun, should.Resemble(map[common.RunID]map[common.CLID]int64{
					run1: {cl.ID: cl.EVersion},
					run2: {cl.ID: cl.EVersion},
				}))
			})

			t.Run("skips an update", func(t *ftt.Test) {
				priorCL := eid.MustCreateIfNotExists(ctx)

				ct.Clock.Add(time.Second)
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return ErrStopMutation
				})
				assert.NoErr(t, err)

				assert.That(t, cl.ExternalID, should.Equal(eid))
				assert.That(t, cl.EVersion, should.Equal(priorCL.EVersion))
				assert.That(t, cl.UpdateTime, should.Match(priorCL.UpdateTime))
				assert.That(t, cl.RetentionKey, should.Equal(priorCL.RetentionKey))

				assert.Loosely(t, pm.byProject, should.BeEmpty)
				assert.Loosely(t, rm.byRun, should.BeEmpty)
				assert.Loosely(t, tj.clsNotified, should.BeEmpty)
			})

			t.Run("propagates error without wrapping", func(t *ftt.Test) {
				myErr := errors.New("my error")
				_, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return myErr
				})
				assert.That(t, myErr, should.Equal(err))
			})
		})

		t.Run("Update method", func(t *ftt.Test) {
			t.Run("updates", func(t *ftt.Test) {
				s1 := makeSnapshot("prior-project", ct.Clock.Now())
				priorCL := eid.MustCreateIfNotExists(ctx)
				priorCL.Snapshot = s1
				assert.Loosely(t, datastore.Put(ctx, priorCL), should.BeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
				s2.MinEquivalentPatchset++
				cl, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					cl.Snapshot = s2
					return nil
				})
				assert.NoErr(t, err)

				assert.Loosely(t, cl.ID, should.Resemble(priorCL.ID))
				assert.That(t, cl.ExternalID, should.Equal(eid))
				assert.Loosely(t, cl.EVersion, should.Equal(2))
				assert.That(t, cl.UpdateTime, should.Match(ct.Clock.Now().UTC()))
				assert.Loosely(t, cl.RetentionKey, should.NotBeEmpty)
				assert.Loosely(t, cl.Snapshot, should.Resemble(s2))

				execBatchOnCLUpdatedTask()
				assert.That(t, pm.byProject[lProject][cl.ID], should.Equal(cl.EVersion))
				assert.Loosely(t, pm.byProject, should.Resemble(map[string]map[common.CLID]int64{
					"prior-project": {cl.ID: cl.EVersion},
					lProject:        {cl.ID: cl.EVersion},
				}))
				assert.Loosely(t, rm.byRun, should.BeEmpty)
				assert.Loosely(t, tj.clsNotified, should.HaveLength(1))
				for clid, content := range tj.clsNotified {
					assert.That(t, clid, should.Equal(cl.ID))
					assert.That(t, content.prevMinEquiPS, should.Equal(s1.GetMinEquivalentPatchset()))
					assert.That(t, content.curMinEquiPS, should.Equal(s2.GetMinEquivalentPatchset()))
				}
			})

			t.Run("schedule tryjob cancel for abandoned CL", func(t *ftt.Test) {
				s1 := makeSnapshot(lProject, ct.Clock.Now())
				cl := eid.MustCreateIfNotExists(ctx)
				cl.Snapshot = s1
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
				s2.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_ABANDONED
				cl, err := m.Update(ctx, lProject, cl.ID, func(cl *CL) error {
					cl.Snapshot = s2
					return nil
				})
				assert.NoErr(t, err)
				assert.Loosely(t, cl.Snapshot, should.Resemble(s2))
				assert.Loosely(t, tj.clsNotified, should.HaveLength(1))
				for clid, content := range tj.clsNotified {
					assert.That(t, clid, should.Equal(cl.ID))
					assert.That(t, content.prevMinEquiPS, should.Equal(s2.GetMinEquivalentPatchset()))
					assert.That(t, content.curMinEquiPS, should.Equal(s2.GetPatchset()+1))
				}
			})

			t.Run("skips an actual update", func(t *ftt.Test) {
				priorCL := eid.MustCreateIfNotExists(ctx)

				ct.Clock.Add(time.Second)
				cl, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					return ErrStopMutation
				})
				assert.NoErr(t, err)

				assert.Loosely(t, cl.ID, should.Resemble(priorCL.ID))
				assert.That(t, cl.ExternalID, should.Equal(eid))
				assert.That(t, cl.EVersion, should.Equal(priorCL.EVersion))
				assert.That(t, cl.UpdateTime, should.Match(priorCL.UpdateTime))
				assert.That(t, cl.RetentionKey, should.Equal(priorCL.RetentionKey))

				expectNoNotifications()
			})

			t.Run("propagates error without wrapping", func(t *ftt.Test) {
				priorCL := eid.MustCreateIfNotExists(ctx)

				myErr := errors.New("my error")
				_, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					return myErr
				})
				assert.That(t, myErr, should.Equal(err))
			})

			t.Run("errors out non-transiently if CL doesn't exist", func(t *ftt.Test) {
				_, err := m.Update(ctx, lProject, 123, func(cl *CL) error {
					panic("must not be called")
				})
				assert.That(t, err, should.ErrLikeError(datastore.ErrNoSuchEntity))
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})
		})

		t.Run("Invalid MutationCallback", func(t *ftt.Test) {
			type badCallback func(cl *CL)
			cases := func(kind string, repro func(bad badCallback) error) {
				t.Run(kind, func(t *ftt.Test) {
					assert.Loosely(t, repro(func(cl *CL) { cl.EVersion = 2 }), should.ErrLike("CL.EVersion"))
					assert.Loosely(t, repro(func(cl *CL) { cl.UpdateTime = ct.Clock.Now() }), should.ErrLike("CL.UpdateTime"))
					assert.Loosely(t, repro(func(cl *CL) { cl.ID++ }), should.ErrLike("CL.ID"))
					assert.Loosely(t, repro(func(cl *CL) { cl.ExternalID = "don't do this" }), should.ErrLike("CL.ExternalID"))
				})
			}
			cases("Upsert creation", func(bad badCallback) error {
				_, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					bad(cl)
					return nil
				})
				return err
			})
			cases("Upsert update", func(bad badCallback) error {
				eid.MustCreateIfNotExists(ctx)
				ct.Clock.Add(time.Second)
				_, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					bad(cl)
					return nil
				})
				return err
			})
			cases("Update", func(bad badCallback) error {
				cl := eid.MustCreateIfNotExists(ctx)
				ct.Clock.Add(time.Second)
				_, err := m.Update(ctx, lProject, cl.ID, func(cl *CL) error {
					bad(cl)
					return nil
				})
				return err
			})
		})
	})
}

func TestMutatorBatch(t *testing.T) {
	t.Parallel()

	ftt.Run("Mutator works on batch of CLs", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProjectAlt = "alt"
		const lProject = "infra"
		const run1 = lProject + "/1"
		const run2 = lProject + "/2"
		const run3 = lProject + "/3"
		const gHost = "x-review.example.com"
		const gChangeFirst = 100000
		const N = 12

		pm := pmMock{}
		rm := rmMock{}
		tj := tjMock{}
		m := NewMutator(ct.TQDispatcher, &pm, &rm, &tj)

		t.Run(fmt.Sprintf("with %d CLs already in Datastore", N), func(t *ftt.Test) {
			var clids common.CLIDs
			var expectedAltProject, expectedRun1, expectedRun2 common.CLIDs
			for gChange := gChangeFirst; gChange < gChangeFirst+N; gChange++ {
				cl := MustGobID(gHost, int64(gChange)).MustCreateIfNotExists(ctx)
				clids = append(clids, cl.ID)
				if gChange%2 == 0 {
					cl.Snapshot = makeSnapshot(lProjectAlt, ct.Clock.Now())
					expectedAltProject = append(expectedAltProject, cl.ID)
				} else {
					cl.Snapshot = makeSnapshot(lProject, ct.Clock.Now())
				}
				if gChange%3 == 0 {
					cl.IncompleteRuns = append(cl.IncompleteRuns, run1)
					expectedRun1 = append(expectedRun1, cl.ID)

				}
				if gChange%5 == 0 {
					cl.IncompleteRuns = append(cl.IncompleteRuns, run2)
					expectedRun2 = append(expectedRun2, cl.ID)
				}
				// Ensure each CL has unique EVersion later on.
				cl.EVersion = int64(10 * gChange)
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
			}
			ct.Clock.Add(time.Minute)

			// In all cases below, run3 is added to the list of incomplete CLs.
			verify := func(resCLs []*CL) {
				// Ensure the returned CLs are exactly what was stored in Datastore,
				// and compute eversion map at the same time.
				dsCLs, err := LoadCLsByIDs(ctx, clids)
				assert.NoErr(t, err)
				eversions := make(map[common.CLID]int64, len(dsCLs))
				for i := range dsCLs {
					assert.Loosely(t, dsCLs[i].IncompleteRuns.ContainsSorted(run3), should.BeTrue)
					assert.That(t, dsCLs[i].ID, should.Equal(resCLs[i].ID))
					assert.That(t, dsCLs[i].EVersion, should.Equal(resCLs[i].EVersion))
					assert.That(t, dsCLs[i].UpdateTime, should.Match(resCLs[i].UpdateTime))
					assert.That(t, dsCLs[i].RetentionKey, should.Equal(resCLs[i].RetentionKey))
					assert.Loosely(t, dsCLs[i].IncompleteRuns, should.Resemble(resCLs[i].IncompleteRuns))
					eversions[dsCLs[i].ID] = dsCLs[i].EVersion
				}

				// Ensure Project and Run managers were notified correctly.
				assertNotified := func(actual map[common.CLID]int64, expectedIDs common.CLIDs) {
					expected := make(map[common.CLID]int64, len(expectedIDs))
					for _, id := range expectedIDs {
						expected[id] = eversions[id]
					}
					assert.Loosely(t, actual, should.Resemble(expected))
				}
				// The project in the context of which CLs were mutated must be notified
				// on all CLs.
				assertNotified(pm.byProject[lProject], clids)
				// Ditto for the run3, which was added to all CLs.
				assertNotified(rm.byRun[run3], clids)
				// Others must be notified on relevant CLs, only.
				assertNotified(pm.byProject[lProjectAlt], expectedAltProject)
				assertNotified(rm.byRun[run1], expectedRun1)
				assertNotified(rm.byRun[run2], expectedRun2)
				assert.Loosely(t, tj.clsNotified, should.BeEmpty)
			}

			t.Run("BeginBatch + FinalizeBatch", func(t *ftt.Test) {
				var resCLs []*CL
				transErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					resCLs = nil // reset in case of retries
					muts, err := m.BeginBatch(ctx, lProject, clids)
					assert.NoErr(t, err)
					eg, _ := errgroup.WithContext(ctx)
					for i := range muts {
						mut := muts[i]
						eg.Go(func() error {
							mut.CL.IncompleteRuns = append(mut.CL.IncompleteRuns, run3)
							return nil
						})
					}
					assert.Loosely(t, eg.Wait(), should.BeNil)
					resCLs, err = m.FinalizeBatch(ctx, muts)
					return err
				}, nil)
				assert.Loosely(t, transErr, should.BeNil)

				// Execute the expected BatchOnCLUpdatedTask.
				assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())

				verify(resCLs)
			})

			t.Run("Manual Adopt + FinalizeBatch", func(t *ftt.Test) {
				var resCLs []*CL
				transErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					resCLs = nil // reset in case of retries
					muts := make([]*CLMutation, len(clids))
					eg, egCtx := errgroup.WithContext(ctx)
					for i, id := range clids {
						i, id := i, id
						eg.Go(func() error {
							cl := &CL{ID: id}
							if err := datastore.Get(egCtx, cl); err != nil {
								return err
							}
							muts[i] = m.Adopt(ctx, lProject, cl)
							muts[i].CL.IncompleteRuns = append(muts[i].CL.IncompleteRuns, run3)
							return nil
						})
					}
					assert.Loosely(t, eg.Wait(), should.BeNil)
					var err error
					resCLs, err = m.FinalizeBatch(ctx, muts)
					return err
				}, nil)
				assert.Loosely(t, transErr, should.BeNil)

				// Execute the expected BatchOnCLUpdatedTask.
				assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())

				verify(resCLs)
			})

			t.Run("BeginBatch + manual finalization", func(t *ftt.Test) {
				// This is inefficient and really shouldn't be done in production.
				var resCLs []*CL
				transErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					resCLs = make([]*CL, len(clids)) // reset in case of retries
					muts, err := m.BeginBatch(ctx, lProject, clids)
					assert.NoErr(t, err)
					eg, egCtx := errgroup.WithContext(ctx)
					for i, mut := range muts {
						i, mut := i, mut
						eg.Go(func() error {
							mut.CL.IncompleteRuns = append(mut.CL.IncompleteRuns, run3)
							var err error
							resCLs[i], err = mut.Finalize(egCtx)
							return err
						})
					}
					return eg.Wait()
				}, nil)
				assert.Loosely(t, transErr, should.BeNil)

				tasks := ct.TQ.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(N))
				for _, tsk := range tasks {
					assert.Loosely(t, tsk.Class, should.Resemble(BatchOnCLUpdatedTaskClass))
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(BatchOnCLUpdatedTaskClass))
				}

				verify(resCLs)
			})
		})
	})
}

func TestMutatorConcurrent(t *testing.T) {
	t.Parallel()

	ftt.Run("Mutator works on single CL when called concurrently with flaky datastore", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		// Truncate to seconds to reduce noise in diffs of proto timestamps.
		// use Seconds with lots of 0s at the end for easy grasp of assertion
		// failures since they are done on protos.
		epoch := (&timestamppb.Timestamp{Seconds: 14500000000}).AsTime()
		ct.Clock.Set(epoch)

		const lProject = "infra"
		const gHost = "x-review.example.com"
		const gChange = 44
		eid := MustGobID(gHost, gChange)

		pm := pmMock{}
		rm := rmMock{}
		tj := tjMock{}
		m := NewMutator(ct.TQDispatcher, &pm, &rm, &tj)

		ctx, fb := featureBreaker.FilterRDS(ctx, nil)
		// Use a single random source for all flaky.Errors(...) instances. Otherwise
		// they repeat the same random pattern each time withBrokenDS is called.
		rnd := rand.NewSource(0)

		// Make datastore very faulty.
		fb.BreakFeaturesWithCallback(
			flaky.Errors(flaky.Params{
				Rand:                             rnd,
				DeadlineProbability:              0.4,
				ConcurrentTransactionProbability: 0.4,
			}),
			featureBreaker.DatastoreFeatures...,
		)
		// Number of tries per worker.
		// With probabilities above, it typically takes <60 tries.
		//
		// This value was set to 300 before 2024-01-16 and it flaked once, so
		// let's increase it to 30000.
		const R = 30000
		// Number of workers.
		const N = 20

		wg := sync.WaitGroup{}
		wg.Add(N)
		for d := 0; d < N; d++ {
			// Simulate each worker trying to update Snapshot and DependentMeta to
			// at least pre-determined timestamp.
			// For extra coverage, use different timestamps for them.

			// For a co-prime p,N:
			//   assert sorted(set([((p*d)%N) for d in xrange(N)])) == range(N)
			// 47, 59 are actual primes.
			snapTS := epoch.Add(time.Second * time.Duration((47*d)%N))
			accTS := epoch.Add(time.Second * time.Duration((73*d)%N))
			go func() {
				defer wg.Done()
				snap := makeSnapshot(lProject, snapTS)
				acc := makeAccess(lProject, accTS)
				var err error
				for i := 0; i < R; i++ {
					// Make this thing a little more robust against flakiness and sleep for a millisecond
					// every so often.
					if i%1000 == 0 {
						time.Sleep(1 * time.Millisecond)
					}
					_, err = m.Upsert(ctx, lProject, eid, func(cl *CL) error {
						ret := ErrStopMutation
						if upTime := cl.Snapshot.GetExternalUpdateTime(); upTime == nil || upTime.AsTime().Before(snapTS) {
							cl.Snapshot = snap
							ret = nil
						}
						if upTime := cl.Access.GetByProject()[lProject].GetUpdateTime(); upTime == nil || upTime.AsTime().Before(accTS) {
							cl.Access = acc
							ret = nil
						}
						return ret
					})
					if err == nil {
						t.Logf("succeeded after %d tries", i)
						return
					}
				}
				panic(errors.Annotate(err, "all %d tries exhausted", R).Err())
			}()
		}
		wg.Wait()

		// "Fix" datastore, letting us examine it.
		fb.BreakFeaturesWithCallback(
			func(context.Context, string) error { return nil },
			featureBreaker.DatastoreFeatures...,
		)
		cl, err := eid.Load(ctx)
		assert.NoErr(t, err)
		assert.Loosely(t, cl, should.NotBeNil)
		// Since all workers have succeeded, the latest snapshot
		// (by ExternalUpdateTime) must be the current snapshot in datastore.
		latestTS := epoch.Add((N - 1) * time.Second)
		assert.That(t, cl.Snapshot.GetExternalUpdateTime().AsTime(), should.Match(latestTS))
		assert.Loosely(t, cl.Access.GetByProject()[lProject].GetUpdateTime().AsTime(), should.Resemble(latestTS))
		// Furthermore, there must have been at most N non-noop UpdateSnapshot calls
		// (one per worker, iff they did it exactly in the increasing order of
		// timestamps.
		t.Logf("%d updates done", cl.EVersion)
		assert.Loosely(t, cl.EVersion, should.BeLessThan(N+1))
	})
}

func makeAccess(luciProject string, updatedTime time.Time) *Access {
	return &Access{ByProject: map[string]*Access_Project{
		luciProject: {
			NoAccess:     true,
			NoAccessTime: timestamppb.New(updatedTime),
			UpdateTime:   timestamppb.New(updatedTime),
		},
	}}
}

type pmMock struct {
	m         sync.Mutex
	byProject map[string]map[common.CLID]int64 // latest max EVersion
}

func (p *pmMock) NotifyCLsUpdated(ctx context.Context, project string, events *CLUpdatedEvents) error {
	p.m.Lock()
	defer p.m.Unlock()
	if p.byProject == nil {
		p.byProject = make(map[string]map[common.CLID]int64, 1)
	}
	m := p.byProject[project]
	if m == nil {
		m = make(map[common.CLID]int64, len(events.GetEvents()))
		p.byProject[project] = m
	}
	for _, e := range events.GetEvents() {
		clid := common.CLID(e.GetClid())
		m[clid] = max(m[clid], e.GetEversion())
	}
	return nil
}

type rmMock struct {
	m     sync.Mutex
	byRun map[common.RunID]map[common.CLID]int64 // latest max EVersion
}

func (r *rmMock) NotifyCLsUpdated(ctx context.Context, rid common.RunID, events *CLUpdatedEvents) error {
	r.m.Lock()
	defer r.m.Unlock()
	if r.byRun == nil {
		r.byRun = make(map[common.RunID]map[common.CLID]int64, 1)
	}
	m := r.byRun[rid]
	if m == nil {
		m = make(map[common.CLID]int64, 1)
		r.byRun[rid] = m
	}
	for _, e := range events.GetEvents() {
		clid := common.CLID(e.GetClid())
		m[clid] = max(m[clid], e.GetEversion())
	}
	return nil
}

type tjMock struct {
	clsNotified map[common.CLID]struct {
		prevMinEquiPS, curMinEquiPS int32
	}
	mutex sync.Mutex
}

func (t *tjMock) ScheduleCancelStale(ctx context.Context, clid common.CLID, prevMinEquivalentPatchset, currentMinEquivalentPatchset int32, eta time.Time) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.clsNotified == nil {
		t.clsNotified = make(map[common.CLID]struct {
			prevMinEquiPS int32
			curMinEquiPS  int32
		})
	}
	t.clsNotified[clid] = struct {
		prevMinEquiPS int32
		curMinEquiPS  int32
	}{
		prevMinEquiPS: prevMinEquivalentPatchset,
		curMinEquiPS:  currentMinEquivalentPatchset,
	}
	return nil
}
