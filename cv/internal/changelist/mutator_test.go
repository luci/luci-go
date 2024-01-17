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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMutatorSingleCL(t *testing.T) {
	t.Parallel()

	Convey("Mutator works on a single CL", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

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
			So(ct.TQ.Tasks(), ShouldHaveLength, 1)
			So(ct.TQ.Tasks()[0].Class, ShouldResemble, BatchOnCLUpdatedTaskClass)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(BatchOnCLUpdatedTaskClass))
		}
		expectNoNotifications := func() {
			So(ct.TQ.Tasks(), ShouldHaveLength, 0)
			So(pm.byProject, ShouldBeEmpty)
			So(rm.byRun, ShouldBeEmpty)
			So(tj.clsNotified, ShouldBeEmpty)
		}

		Convey("Upsert method", func() {
			Convey("creates", func() {
				s := makeSnapshot(lProject, ct.Clock.Now())
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					cl.Snapshot = s
					return nil
				})
				So(err, ShouldBeNil)
				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, 1)
				So(cl.UpdateTime, ShouldEqual, ct.Clock.Now().UTC())
				So(cl.Snapshot, ShouldResembleProto, s)

				execBatchOnCLUpdatedTask()
				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int64{
					lProject: {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldBeEmpty)
				So(tj.clsNotified, ShouldBeEmpty)
			})

			Convey("skips creation", func() {
				// This is a special case which isn't supposed to be needed,
				// but it's kept here for completeness.
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return ErrStopMutation
				})
				So(err, ShouldBeNil)
				So(cl, ShouldBeNil)
				expectNoNotifications()
			})

			Convey("updates", func() {
				s1 := makeSnapshot(lProject, ct.Clock.Now())
				cl := eid.MustCreateIfNotExists(ctx)
				cl.Snapshot = s1
				cl.IncompleteRuns = common.MakeRunIDs(run1)
				So(datastore.Put(ctx, cl), ShouldBeNil)

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
				So(err, ShouldBeNil)

				So(priorSnapshot, ShouldResembleProto, s1)
				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, 2)
				So(cl.UpdateTime, ShouldEqual, ct.Clock.Now().UTC())
				So(cl.Snapshot, ShouldResembleProto, s2)
				So(tj.clsNotified[0], ShouldEqual, cl.ID)

				execBatchOnCLUpdatedTask()
				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int64{
					lProject: {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldResemble, map[common.RunID]map[common.CLID]int64{
					run1: {cl.ID: cl.EVersion},
					run2: {cl.ID: cl.EVersion},
				})
			})

			Convey("skips an update", func() {
				priorCL := eid.MustCreateIfNotExists(ctx)

				ct.Clock.Add(time.Second)
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return ErrStopMutation
				})
				So(err, ShouldBeNil)

				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, priorCL.EVersion)
				So(cl.UpdateTime, ShouldEqual, priorCL.UpdateTime)

				So(pm.byProject, ShouldBeEmpty)
				So(rm.byRun, ShouldBeEmpty)
				So(tj.clsNotified, ShouldBeEmpty)
			})

			Convey("propagates error without wrapping", func() {
				myErr := errors.New("my error")
				_, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return myErr
				})
				So(myErr, ShouldEqual, err)
			})
		})

		Convey("Update method", func() {
			Convey("updates", func() {
				s1 := makeSnapshot("prior-project", ct.Clock.Now())
				priorCL := eid.MustCreateIfNotExists(ctx)
				priorCL.Snapshot = s1
				So(datastore.Put(ctx, priorCL), ShouldBeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
				s2.MinEquivalentPatchset++
				cl, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					cl.Snapshot = s2
					return nil
				})
				So(err, ShouldBeNil)

				So(cl.ID, ShouldResemble, priorCL.ID)
				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, 2)
				So(cl.UpdateTime, ShouldEqual, ct.Clock.Now().UTC())
				So(cl.Snapshot, ShouldResembleProto, s2)

				execBatchOnCLUpdatedTask()
				So(pm.byProject[lProject][cl.ID], ShouldEqual, cl.EVersion)
				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int64{
					"prior-project": {cl.ID: cl.EVersion},
					lProject:        {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldBeEmpty)
				So(tj.clsNotified[0], ShouldEqual, cl.ID)
			})

			Convey("skips an actual update", func() {
				priorCL := eid.MustCreateIfNotExists(ctx)

				ct.Clock.Add(time.Second)
				cl, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					return ErrStopMutation
				})
				So(err, ShouldBeNil)

				So(cl.ID, ShouldResemble, priorCL.ID)
				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, priorCL.EVersion)
				So(cl.UpdateTime, ShouldEqual, priorCL.UpdateTime)

				expectNoNotifications()
			})

			Convey("propagates error without wrapping", func() {
				priorCL := eid.MustCreateIfNotExists(ctx)

				myErr := errors.New("my error")
				_, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					return myErr
				})
				So(myErr, ShouldEqual, err)
			})

			Convey("errors out non-transiently if CL doesn't exist", func() {
				_, err := m.Update(ctx, lProject, 123, func(cl *CL) error {
					panic("must not be called")
				})
				So(errors.Unwrap(err), ShouldEqual, datastore.ErrNoSuchEntity)
				So(transient.Tag.In(err), ShouldBeFalse)
			})
		})

		Convey("Invalid MutationCallback", func() {
			type badCallback func(cl *CL)
			cases := func(kind string, repro func(bad badCallback)) {
				Convey(kind, func() {
					So(func() { repro(func(cl *CL) { cl.EVersion = 2 }) }, ShouldPanicLike, "CL.EVersion")
					So(func() { repro(func(cl *CL) { cl.UpdateTime = ct.Clock.Now() }) }, ShouldPanicLike, "CL.UpdateTime")
					So(func() { repro(func(cl *CL) { cl.ID++ }) }, ShouldPanicLike, "CL.ID")
					So(func() { repro(func(cl *CL) { cl.ExternalID = "don't do this" }) }, ShouldPanicLike, "CL.ExternalID")
				})
			}
			cases("Upsert creation", func(bad badCallback) {
				_, _ = m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					bad(cl)
					return nil
				})
			})
			cases("Upsert update", func(bad badCallback) {
				eid.MustCreateIfNotExists(ctx)
				ct.Clock.Add(time.Second)
				_, _ = m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					bad(cl)
					return nil
				})
			})
			cases("Update", func(bad badCallback) {
				cl := eid.MustCreateIfNotExists(ctx)
				ct.Clock.Add(time.Second)
				_, _ = m.Update(ctx, lProject, cl.ID, func(cl *CL) error {
					bad(cl)
					return nil
				})
			})
		})
	})
}

func TestMutatorBatch(t *testing.T) {
	t.Parallel()

	Convey("Mutator works on batch of CLs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

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

		Convey(fmt.Sprintf("with %d CLs already in Datastore", N), func() {
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
				So(datastore.Put(ctx, cl), ShouldBeNil)
			}
			ct.Clock.Add(time.Minute)

			// In all cases below, run3 is added to the list of incomplete CLs.
			verify := func(resCLs []*CL) {
				// Ensure the returned CLs are exactly what was stored in Datastore,
				// and compute eversion map at the same time.
				dsCLs, err := LoadCLsByIDs(ctx, clids)
				So(err, ShouldBeNil)
				eversions := make(map[common.CLID]int64, len(dsCLs))
				for i := range dsCLs {
					So(dsCLs[i].IncompleteRuns.ContainsSorted(run3), ShouldBeTrue)
					So(dsCLs[i].ID, ShouldEqual, resCLs[i].ID)
					So(dsCLs[i].EVersion, ShouldEqual, resCLs[i].EVersion)
					So(dsCLs[i].UpdateTime, ShouldResemble, resCLs[i].UpdateTime)
					So(dsCLs[i].IncompleteRuns, ShouldResemble, resCLs[i].IncompleteRuns)
					eversions[dsCLs[i].ID] = dsCLs[i].EVersion
				}

				// Ensure Project and Run managers were notified correctly.
				assertNotified := func(actual map[common.CLID]int64, expectedIDs common.CLIDs) {
					expected := make(map[common.CLID]int64, len(expectedIDs))
					for _, id := range expectedIDs {
						expected[id] = eversions[id]
					}
					So(actual, ShouldResemble, expected)
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
				So(tj.clsNotified.Set(), ShouldBeEmpty)
			}

			Convey("BeginBatch + FinalizeBatch", func() {
				var resCLs []*CL
				transErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					resCLs = nil // reset in case of retries
					muts, err := m.BeginBatch(ctx, lProject, clids)
					So(err, ShouldBeNil)
					eg, _ := errgroup.WithContext(ctx)
					for i := range muts {
						mut := muts[i]
						eg.Go(func() error {
							mut.CL.IncompleteRuns = append(mut.CL.IncompleteRuns, run3)
							return nil
						})
					}
					So(eg.Wait(), ShouldBeNil)
					resCLs, err = m.FinalizeBatch(ctx, muts)
					return err
				}, nil)
				So(transErr, ShouldBeNil)

				// Execute the expected BatchOnCLUpdatedTask.
				So(ct.TQ.Tasks(), ShouldHaveLength, 1)
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())

				verify(resCLs)
			})

			Convey("Manual Adopt + FinalizeBatch", func() {
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
					So(eg.Wait(), ShouldBeNil)
					var err error
					resCLs, err = m.FinalizeBatch(ctx, muts)
					return err
				}, nil)
				So(transErr, ShouldBeNil)

				// Execute the expected BatchOnCLUpdatedTask.
				So(ct.TQ.Tasks(), ShouldHaveLength, 1)
				ct.TQ.Run(ctx, tqtesting.StopWhenDrained())

				verify(resCLs)
			})

			Convey("BeginBatch + manual finalization", func() {
				// This is inefficient and really shouldn't be done in production.
				var resCLs []*CL
				transErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					resCLs = make([]*CL, len(clids)) // reset in case of retries
					muts, err := m.BeginBatch(ctx, lProject, clids)
					So(err, ShouldBeNil)
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
				So(transErr, ShouldBeNil)

				tasks := ct.TQ.Tasks()
				So(tasks, ShouldHaveLength, N)
				for _, t := range tasks {
					So(t.Class, ShouldResemble, BatchOnCLUpdatedTaskClass)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(BatchOnCLUpdatedTaskClass))
				}

				verify(resCLs)
			})
		})
	})
}

func TestMutatorConcurrent(t *testing.T) {
	t.Parallel()

	Convey("Mutator works on single CL when called concurrently with flaky datastore", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
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
					if i % 1000 == 0 {
						time.Sleep(1 * time.Millisecond)
					}
					_, err = m.Upsert(ctx, lProject, eid, func(cl *CL) error {
						ret := ErrStopMutation
						if t := cl.Snapshot.GetExternalUpdateTime(); t == nil || t.AsTime().Before(snapTS) {
							cl.Snapshot = snap
							ret = nil
						}
						if t := cl.Access.GetByProject()[lProject].GetUpdateTime(); t == nil || t.AsTime().Before(accTS) {
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
		So(err, ShouldBeNil)
		So(cl, ShouldNotBeNil)
		// Since all workers have succeeded, the latest snapshot
		// (by ExternalUpdateTime) must be the current snapshot in datastore.
		latestTS := epoch.Add((N - 1) * time.Second)
		So(cl.Snapshot.GetExternalUpdateTime().AsTime(), ShouldEqual, latestTS)
		So(cl.Access.GetByProject()[lProject].GetUpdateTime().AsTime(), ShouldResemble, latestTS)
		// Furthermore, there must have been at most N non-noop UpdateSnapshot calls
		// (one per worker, iff they did it exactly in the increasing order of
		// timestamps.
		t.Logf("%d updates done", cl.EVersion)
		So(cl.EVersion, ShouldBeLessThan, N+1)
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
	clsNotified common.CLIDs
	mutex       sync.Mutex
}

func (t *tjMock) ScheduleCancelStale(ctx context.Context, clid common.CLID, prevMinEquivalentPatchset, currentMinEquivalentPatchset int32, eta time.Time) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.clsNotified = append(t.clsNotified, clid)
	return nil
}
