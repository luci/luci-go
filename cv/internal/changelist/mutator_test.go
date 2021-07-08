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
	"math/rand"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMutatorSingleCL(t *testing.T) {
	t.Parallel()

	Convey("Mutator works on a single CL", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		// Truncate to seconds to reduce noise in diffs of proto timestamps.
		// use Seconds with lots of 0s at the end for easy grasp of assertion
		// failures since they are done on protos.
		epoch := (&timestamppb.Timestamp{Seconds: 14500000000}).AsTime()
		ct.Clock.Set(epoch)

		const lProject = "infra"
		const run1 = lProject + "/1"
		const run2 = lProject + "/2"
		const gHost = "x-review.example.com"
		const gChange = 44
		eid := MustGobID(gHost, gChange)

		pm := pmMock{}
		rm := rmMock{}
		m := NewMutator(&pm, &rm)

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

				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int{
					lProject: {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldBeEmpty)
			})

			Convey("skips creation", func() {
				// This is a special case which isn't supposed to be needed,
				// but it's kept here for completness.
				cl, err := m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					return ErrStopMutation
				})
				So(err, ShouldBeNil)
				So(cl, ShouldBeNil)
				So(pm.byProject, ShouldBeEmpty)
				So(rm.byRun, ShouldBeEmpty)
			})

			Convey("updates", func() {
				s1 := makeSnapshot(lProject, ct.Clock.Now())
				_, err := eid.GetOrInsert(ctx, func(cl *CL) {
					cl.Snapshot = s1
					cl.IncompleteRuns = common.MakeRunIDs(run1)
				})
				So(err, ShouldBeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
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

				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int{
					lProject: {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldResemble, map[common.RunID]map[common.CLID]int{
					run1: {cl.ID: cl.EVersion},
					run2: {cl.ID: cl.EVersion},
				})
			})

			Convey("skips an update", func() {
				priorCL, err := eid.GetOrInsert(ctx, func(cl *CL) {})
				So(err, ShouldBeNil)

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
				priorCL, err := eid.GetOrInsert(ctx, func(cl *CL) {
					cl.Snapshot = s1
				})
				So(err, ShouldBeNil)

				ct.Clock.Add(time.Second)
				s2 := makeSnapshot(lProject, ct.Clock.Now())
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

				So(pm.byProject[lProject][cl.ID], ShouldEqual, cl.EVersion)
				So(pm.byProject, ShouldResemble, map[string]map[common.CLID]int{
					"prior-project": {cl.ID: cl.EVersion},
					lProject:        {cl.ID: cl.EVersion},
				})
				So(rm.byRun, ShouldBeEmpty)
			})

			Convey("skips an actual update", func() {
				priorCL, err := eid.GetOrInsert(ctx, func(cl *CL) {})
				So(err, ShouldBeNil)

				ct.Clock.Add(time.Second)
				cl, err := m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
					return ErrStopMutation
				})
				So(err, ShouldBeNil)

				So(cl.ID, ShouldResemble, priorCL.ID)
				So(cl.ExternalID, ShouldResemble, eid)
				So(cl.EVersion, ShouldEqual, priorCL.EVersion)
				So(cl.UpdateTime, ShouldEqual, priorCL.UpdateTime)

				So(pm.byProject, ShouldBeEmpty)
				So(rm.byRun, ShouldBeEmpty)
			})

			Convey("propagates error without wrapping", func() {
				priorCL, err := eid.GetOrInsert(ctx, func(cl *CL) {})
				So(err, ShouldBeNil)

				myErr := errors.New("my error")
				_, err = m.Update(ctx, lProject, priorCL.ID, func(cl *CL) error {
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
				_, err := eid.GetOrInsert(ctx, func(cl *CL) {})
				So(err, ShouldBeNil)
				ct.Clock.Add(time.Second)
				_, _ = m.Upsert(ctx, lProject, eid, func(cl *CL) error {
					bad(cl)
					return nil
				})
			})
			cases("Update", func(bad badCallback) {
				cl, err := eid.GetOrInsert(ctx, func(cl *CL) {})
				So(err, ShouldBeNil)
				ct.Clock.Add(time.Second)
				_, _ = m.Update(ctx, lProject, cl.ID, func(cl *CL) error {
					bad(cl)
					return nil
				})
			})
		})
	})
}

func TestMutatorConcurrent(t *testing.T) {
	t.Parallel()

	Convey("Mutator works on single CL when called concurrently with flaky datastore", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		// Truncate to seconds to reduce noise in diffs of proto timestamps.
		// use Seconds with lots of 0s at the end for easy grasp of assertion
		// failures since they are done on protos.
		epoch := (&timestamppb.Timestamp{Seconds: 14500000000}).AsTime()
		ct.Clock.Set(epoch)

		const lProject = "infra"
		const run1 = lProject + "/1"
		const run2 = lProject + "/2"
		const gHost = "x-review.example.com"
		const gChange = 44
		eid := MustGobID(gHost, gChange)

		pm := pmMock{}
		rm := rmMock{}
		m := NewMutator(&pm, &rm)

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
		const R = 200
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
		cl, err := eid.Get(ctx)
		So(err, ShouldBeNil)
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
	byProject map[string]map[common.CLID]int // latest max EVersion
}

func (p *pmMock) NotifyCLUpdated(ctx context.Context, project string, cl common.CLID, eversion int) error {
	p.m.Lock()
	defer p.m.Unlock()
	if p.byProject == nil {
		p.byProject = make(map[string]map[common.CLID]int, 1)
	}
	m := p.byProject[project]
	if m == nil {
		m = make(map[common.CLID]int, 1)
		p.byProject[project] = m
	}
	m[cl] = max(m[cl], eversion)
	return nil
}

type rmMock struct {
	m     sync.Mutex
	byRun map[common.RunID]map[common.CLID]int // latest max EVersion
}

func (r *rmMock) NotifyCLUpdated(ctx context.Context, run common.RunID, cl common.CLID, eversion int) error {
	r.m.Lock()
	defer r.m.Unlock()
	if r.byRun == nil {
		r.byRun = make(map[common.RunID]map[common.CLID]int, 1)
	}
	m := r.byRun[run]
	if m == nil {
		m = make(map[common.CLID]int, 1)
		r.byRun[run] = m
	}
	m[cl] = max(m[cl], eversion)
	return nil
}

func max(i, j int) int {
	if i < j {
		return j
	}
	return i
}
