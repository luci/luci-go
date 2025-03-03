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

package tjcancel

import (
	"context"
	"crypto/sha1"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestTaskHandler(t *testing.T) {
	ftt.Run("handleTask", t, func(t *ftt.Test) {
		t.Run("panics", func(t *ftt.Test) {
			c := &Cancellator{}
			ctx := context.Background()

			panicker := func() {
				_ = c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     42,
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  2,
				})
			}
			assert.Loosely(t, panicker, should.PanicLike("patchset numbers expected to increase"))
		})
		t.Run("works with", func(t *ftt.Test) {
			ct := &cvtesting.Test{}
			ctx := ct.SetUp(t)
			n := tryjob.NewNotifier(ct.TQDispatcher)
			c := NewCancellator(n)
			mb := &mockBackend{}
			c.RegisterBackend(mb)
			const clid = common.CLID(100)
			cl := &changelist.CL{ID: clid}
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
			t.Run("no tryjobs", func(t *ftt.Test) {
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))
			})
			t.Run("all tryjobs ended", func(t *ftt.Test) {
				tj1 := putTryjob(t, ctx, clid, 2, tryjob.Status_ENDED, 1, run.Status_FAILED, nil)
				tj2 := putTryjob(t, ctx, clid, 2, tryjob.Status_ENDED, 2, run.Status_CANCELLED, nil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				// Should not call backend.
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))

				assert.Loosely(t, datastore.Get(ctx, tj1, tj2), should.BeNil)
				// Should not modify entities.
				assert.Loosely(t, tj1.EVersion, should.Equal(1))
				assert.Loosely(t, tj2.EVersion, should.Equal(1))
			})
			t.Run("some tryjobs ended, others cancellable", func(t *ftt.Test) {
				tj11 := putTryjob(t, ctx, clid, 2, tryjob.Status_ENDED, 11, run.Status_FAILED, nil)
				tj12 := putTryjob(t, ctx, clid, 2, tryjob.Status_TRIGGERED, 12, run.Status_CANCELLED, nil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				// Should call backend once, with tj12.
				assert.Loosely(t, mb.calledWith, should.HaveLength(1))
				assert.Loosely(t, mb.calledWith[0].ExternalID, should.Equal(tj12.ExternalID))

				assert.Loosely(t, datastore.Get(ctx, tj11, tj12), should.BeNil)
				// Should modify only tj12.
				assert.Loosely(t, tj11.EVersion, should.Equal(1))
				assert.Loosely(t, tj12.EVersion, should.Equal(2))
				assert.Loosely(t, tj12.Status, should.Equal(tryjob.Status_CANCELLED))
			})
			t.Run("tryjob still watched", func(t *ftt.Test) {
				tj21 := putTryjob(t, ctx, clid, 2, tryjob.Status_TRIGGERED, 21, run.Status_RUNNING, nil)
				task := &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				}
				err := c.handleTask(ctx, task)
				assert.NoErr(t, err)
				// Should not call backend.
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))

				assert.Loosely(t, datastore.Get(ctx, tj21), should.BeNil)
				// Should not modify the entity.
				assert.Loosely(t, tj21.EVersion, should.Equal(1))
				assert.Loosely(t, tj21.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
				assert.Loosely(t, ct.TQ.Tasks()[0].Payload, should.HaveType[*tryjob.CancelStaleTryjobsTask])
				assert.That(t, ct.TQ.Tasks()[0].Payload.(*tryjob.CancelStaleTryjobsTask), should.Match(task))
				assert.That(t, ct.TQ.Tasks()[0].ETA, should.Match(ct.Clock.Now().Add(cancelLaterDuration)))
			})
			t.Run("tryjob not triggered by cv", func(t *ftt.Test) {
				tj31 := putTryjob(t, ctx, clid, 2, tryjob.Status_TRIGGERED, 31, run.Status_CANCELLED, func(tj *tryjob.Tryjob) {
					tj.LaunchedBy = ""
				})
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				// Should not call backend.
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))
				assert.Loosely(t, datastore.Get(ctx, tj31), should.BeNil)
				// Should not modify the entity.
				assert.Loosely(t, tj31.EVersion, should.Equal(1))
				assert.Loosely(t, tj31.Status, should.NotEqual(tryjob.Status_CANCELLED))
			})
			t.Run("tryjob configured to skip stale check", func(t *ftt.Test) {
				tj41 := putTryjob(t, ctx, clid, 2, tryjob.Status_TRIGGERED, 41, run.Status_CANCELLED, func(tj *tryjob.Tryjob) {
					tj.Definition.SkipStaleCheck = true
				})
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				// Should not call backend.
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))
				assert.Loosely(t, datastore.Get(ctx, tj41), should.BeNil)
				// Should not modify the entity.
				assert.Loosely(t, tj41.EVersion, should.Equal(1))
				assert.Loosely(t, tj41.Status, should.NotEqual(tryjob.Status_CANCELLED))
			})
			t.Run("CL has Cq-Do-Not-Cancel-Tryjobs footer", func(t *ftt.Test) {
				tj51 := putTryjob(t, ctx, clid, 2, tryjob.Status_TRIGGERED, 12, run.Status_CANCELLED, nil)
				cl.Snapshot = &changelist.Snapshot{
					Metadata: []*changelist.StringPair{
						{
							Key:   common.FooterCQDoNotCancelTryjobs,
							Value: "True",
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				assert.NoErr(t, err)
				// Should not call backend.
				assert.Loosely(t, mb.calledWith, should.HaveLength(0))
				assert.Loosely(t, datastore.Get(ctx, tj51), should.BeNil)
				// Should not modify the entity.
				assert.Loosely(t, tj51.EVersion, should.Equal(1))
				assert.Loosely(t, tj51.Status, should.NotEqual(tryjob.Status_CANCELLED))
			})
		})
	})
}

// putTryjob creates a mock Tryjob and its triggering Run.
//
// It must be called inside a Convey() context as it contains
// assertions.
func putTryjob(t testing.TB, ctx context.Context, clid common.CLID, patchset int32, tjStatus tryjob.Status, buildNumber int64, runStatus run.Status, modify func(*tryjob.Tryjob)) *tryjob.Tryjob {
	t.Helper()

	now := datastore.RoundTime(clock.Now(ctx).UTC())
	tjID := tryjob.MustBuildbucketID("test.com", buildNumber)
	digest := mockDigest(string(tjID))
	r := &run.Run{
		ID:     common.MakeRunID("test", now, 1, digest),
		Status: runStatus,
	}
	assert.Loosely(t, datastore.Put(ctx, r), should.BeNil, truth.LineContext())
	tj := &tryjob.Tryjob{
		ExternalID:       tjID,
		CLPatchsets:      []tryjob.CLPatchset{tryjob.MakeCLPatchset(clid, patchset)},
		Status:           tjStatus,
		EVersion:         1,
		EntityCreateTime: now,
		EntityUpdateTime: now,
		LaunchedBy:       r.ID,
		Definition:       &tryjob.Definition{},
	}
	if modify != nil {
		modify(tj)
	}
	assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil, truth.LineContext())
	return tj
}

// mockDigest hashes a string.
func mockDigest(s string) []byte {
	h := sha1.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}

type mockBackend struct {
	calledWith []*tryjob.Tryjob
}

func (mb *mockBackend) Kind() string {
	return "buildbucket"
}

func (mb *mockBackend) CancelTryjob(ctx context.Context, tj *tryjob.Tryjob, reason string) error {
	mb.calledWith = append(mb.calledWith, tj)
	return nil
}
