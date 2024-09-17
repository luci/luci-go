// Copyright 2024 The LUCI Authors.
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

package tryjob

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestUpsert(t *testing.T) {
	t.Parallel()

	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	rm := &rmMock{}
	m := NewMutator(rm)
	eid := MustBuildbucketID("example.bb.com", 1000)
	t.Run("Create", func(t *testing.T) {
		tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
			tj.Status = Status_PENDING
			return nil
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.ID, should.NotEqual(common.TryjobID(0)))
		assert.That(t, tj.ExternalID, should.Equal(eid))
		assert.That(t, tj.EVersion, should.Equal(int64(1)))
		assert.That(t, tj.EntityCreateTime, should.Match(datastore.RoundTime(ct.Clock.Now())))
		assert.That(t, tj.EntityUpdateTime, should.Match(datastore.RoundTime(ct.Clock.Now())))
		assert.That(t, tj.Status, should.Equal(Status_PENDING))
	})
	t.Run("Skips creation", func(t *testing.T) {
		// This is a special case which isn't supposed to be needed,
		// but it's kept here for completeness.
		tj, err := m.Upsert(ctx, MustBuildbucketID("example.bb.com", 1001), func(tj *Tryjob) error {
			return ErrStopMutation
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, tj, should.BeNil)
	})
	t.Run("Updates", func(t *testing.T) {
		ct.Clock.Add(1 * time.Minute)
		tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
			tj.Status = Status_TRIGGERED
			return nil
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.ExternalID, should.Equal(eid))
		assert.That(t, tj.EVersion, should.Equal(int64(2)))
		assert.That(t, tj.EntityCreateTime, should.Match(datastore.RoundTime(ct.Clock.Now().Add(-1*time.Minute))))
		assert.That(t, tj.EntityUpdateTime, should.Match(datastore.RoundTime(ct.Clock.Now())))
		assert.That(t, tj.Status, should.Equal(Status_TRIGGERED))
	})
	t.Run("Skips update", func(t *testing.T) {
		ct.Clock.Add(1 * time.Minute)
		tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
			return ErrStopMutation
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.EVersion, should.Equal(int64(2)))
		assert.That(t, tj.EntityUpdateTime, should.Match(datastore.RoundTime(ct.Clock.Now().Add(-1*time.Minute))))
	})
	t.Run("Notify runs", func(t *testing.T) {
		runID1 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("cafe"))
		runID2 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("beef"))
		tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
			tj.LaunchedBy = runID1
			tj.ReusedBy = append(tj.ReusedBy, runID2)
			return nil
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.LaunchedBy, should.Equal(runID1))
		assert.That(t, tj.ReusedBy, should.Match(common.RunIDs{runID2}))
		assert.That(t, rm.notified, should.Match(map[common.RunID]common.TryjobIDs{
			runID1: {tj.ID},
			runID2: {tj.ID},
		}))
	})
	t.Run("MutatorCallback error", func(t *testing.T) {
		myErr := errors.New("my error")
		_, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
			return myErr
		})
		assert.That(t, err, should.ErrLike(myErr))
	})
	t.Run("MutatorCallback modify immutable fields", func(t *testing.T) {
		assert.That(t, func() {
			m.Upsert(ctx, eid, func(tj *Tryjob) error {
				tj.EVersion = 100
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Upsert(ctx, eid, func(tj *Tryjob) error {
				tj.ExternalID = MustBuildbucketID("example.bb.com", 999)
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Upsert(ctx, eid, func(tj *Tryjob) error {
				tj.EntityCreateTime = ct.Clock.Now().Add(1 * time.Hour)
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Upsert(ctx, eid, func(tj *Tryjob) error {
				tj.EntityUpdateTime = ct.Clock.Now().Add(1 * time.Hour)
				return nil
			})
		}, should.Panic)
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	rm := &rmMock{}
	m := NewMutator(rm)
	tj, err := m.Upsert(ctx, MustBuildbucketID("example.bb.com", 2000), func(tj *Tryjob) error {
		return nil
	})
	assert.That(t, err, should.ErrLike(nil))
	tryjobID := tj.ID
	ct.Clock.Add(1 * time.Minute)
	t.Run("Update", func(t *testing.T) {
		tj, err := m.Update(ctx, tryjobID, func(tj *Tryjob) error {
			tj.Status = Status_UNTRIGGERED
			tj.UntriggeredReason = "bad"
			return nil
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.ID, should.Equal(tryjobID))
		assert.That(t, tj.EVersion, should.Equal(int64(2)))
		assert.That(t, tj.EntityUpdateTime, should.Match(datastore.RoundTime(ct.Clock.Now())))
		assert.That(t, tj.Status, should.Equal(Status_UNTRIGGERED))
		assert.That(t, tj.UntriggeredReason, should.Equal("bad"))
	})

	t.Run("Update External ID", func(t *testing.T) {
		t.Run("No mapping exists", func(t *testing.T) {
			newTryjob := &Tryjob{}
			assert.That(t, datastore.Put(ctx, newTryjob), should.ErrLike(nil))
			newExternalID := MustBuildbucketID("example.bb.com", 999)
			newTryjob, err := m.Update(ctx, newTryjob.ID, func(tj *Tryjob) error {
				tj.ExternalID = newExternalID
				return nil
			})
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, newTryjob.ExternalID, should.Equal(newExternalID))
			resolved, err := newExternalID.Resolve(ctx)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, resolved, should.Equal(newTryjob.ID))
		})
		t.Run("Conflicting mapping exists", func(t *testing.T) {
			newTryjob := &Tryjob{}
			assert.That(t, datastore.Put(ctx, newTryjob), should.ErrLike(nil))
			eid := tj.ExternalID // already maps to another Tryjob
			_, err := m.Update(ctx, newTryjob.ID, func(tj *Tryjob) error {
				tj.ExternalID = eid
				return nil
			})
			assert.Loosely(t, err, should.NotBeNil)
			cErr := &ConflictTryjobsError{}
			assert.That(t, errors.As(err, &cErr), should.BeTrue)
			assert.That(t, cErr, should.Match(&ConflictTryjobsError{
				ExternalID: eid,
				Existing:   tj.ID,
				Intended:   newTryjob.ID,
			}))
		})
	})

	t.Run("Skips update", func(t *testing.T) {
		ct.Clock.Add(1 * time.Minute)
		tj, err := m.Update(ctx, tryjobID, func(tj *Tryjob) error {
			return ErrStopMutation
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.EVersion, should.Equal(int64(2)))
		assert.That(t, tj.EntityUpdateTime, should.Match(datastore.RoundTime(ct.Clock.Now().Add(-1*time.Minute))))
	})

	t.Run("Notify runs", func(t *testing.T) {
		runID1 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("cafe"))
		runID2 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("beef"))
		tj, err := m.Update(ctx, tryjobID, func(tj *Tryjob) error {
			tj.LaunchedBy = runID1
			tj.ReusedBy = append(tj.ReusedBy, runID2)
			return nil
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, tj.LaunchedBy, should.Equal(runID1))
		assert.That(t, tj.ReusedBy, should.Match(common.RunIDs{runID2}))
		assert.That(t, rm.notified, should.Match(map[common.RunID]common.TryjobIDs{
			runID1: {tj.ID},
			runID2: {tj.ID},
		}))
	})
	t.Run("Tryjob doesn't exist", func(t *testing.T) {
		_, err := m.Update(ctx, tryjobID+1000, func(tj *Tryjob) error {
			return nil
		})
		assert.That(t, err, should.ErrLikeError(datastore.ErrNoSuchEntity))
	})
	t.Run("MutatorCallback error", func(t *testing.T) {
		myErr := errors.New("my error")
		_, err := m.Update(ctx, tryjobID, func(tj *Tryjob) error {
			return myErr
		})
		assert.That(t, err, should.ErrLike(myErr))
	})
	t.Run("MutatorCallback modify immutable fields", func(t *testing.T) {
		assert.That(t, func() {
			m.Update(ctx, tryjobID, func(tj *Tryjob) error {
				tj.EVersion = 100
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Update(ctx, tryjobID, func(tj *Tryjob) error {
				tj.ExternalID = MustBuildbucketID("example.bb.com", 999)
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Update(ctx, tryjobID, func(tj *Tryjob) error {
				tj.EntityCreateTime = ct.Clock.Now().Add(1 * time.Hour)
				return nil
			})
		}, should.Panic)
		assert.That(t, func() {
			m.Update(ctx, tryjobID, func(tj *Tryjob) error {
				tj.EntityUpdateTime = ct.Clock.Now().Add(1 * time.Hour)
				return nil
			})
		}, should.Panic)
	})
}

func TestMutatorBatch(t *testing.T) {
	t.Parallel()

	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)

	t.Run("BeginBatch and FinalizeBatch", func(t *testing.T) {
		m := NewMutator(&rmMock{})
		tryjobIDs := make(common.TryjobIDs, 3)
		for i := range tryjobIDs {
			eid := MustBuildbucketID("example.bb.com", int64(5000+i))
			tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
				return nil
			})
			assert.That(t, err, should.ErrLike(nil))
			tryjobIDs[i] = tj.ID
		}
		var tryjobs []*Tryjob
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			muts, err := m.BeginBatch(ctx, tryjobIDs)
			if err != nil {
				return err
			}
			for _, mut := range muts {
				mut.Tryjob.Status = Status_PENDING
			}
			tryjobs, err = m.FinalizeBatch(ctx, muts)
			return err
		}, nil)
		assert.That(t, err, should.ErrLike(nil))

		for _, tj := range tryjobs {
			assert.That(t, tj.EVersion, should.Equal(int64(2)))
			assert.That(t, tj.Status, should.Equal(Status_PENDING))
		}
	})
	t.Run("Non-batch begin andFinalizeBatch", func(t *testing.T) {
		m := NewMutator(&rmMock{})
		tryjobIDs := make(common.TryjobIDs, 3)
		for i := range tryjobIDs {
			eid := MustBuildbucketID("example.bb.com", int64(6000+i))
			tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
				return nil
			})
			assert.That(t, err, should.ErrLike(nil))
			tryjobIDs[i] = tj.ID
		}
		var tryjobs []*Tryjob
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
			muts := make([]*TryjobMutation, len(tryjobIDs))
			for i, tryjobID := range tryjobIDs {
				if muts[i], err = m.Begin(ctx, tryjobID); err != nil {
					return err
				}
			}
			for _, mut := range muts {
				mut.Tryjob.Status = Status_PENDING
			}
			tryjobs, err = m.FinalizeBatch(ctx, muts)
			return err
		}, nil)
		assert.That(t, err, should.ErrLike(nil))

		for _, tj := range tryjobs {
			assert.That(t, tj.EVersion, should.Equal(int64(2)))
			assert.That(t, tj.Status, should.Equal(Status_PENDING))
		}
	})

	t.Run("BeginBatch and non-batch finalization", func(t *testing.T) {
		m := NewMutator(&rmMock{})
		tryjobIDs := make(common.TryjobIDs, 3)
		for i := range tryjobIDs {
			eid := MustBuildbucketID("example.bb.com", int64(7000+i))
			tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
				return nil
			})
			assert.That(t, err, should.ErrLike(nil))
			tryjobIDs[i] = tj.ID
		}
		var tryjobs []*Tryjob
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			muts, err := m.BeginBatch(ctx, tryjobIDs)
			if err != nil {
				return err
			}
			for _, mut := range muts {
				mut.Tryjob.Status = Status_PENDING
				tj, err := mut.Finalize(ctx)
				if err != nil {
					return err
				}
				tryjobs = append(tryjobs, tj)
			}
			return nil
		}, nil)
		assert.That(t, err, should.ErrLike(nil))

		for _, tj := range tryjobs {
			assert.That(t, tj.EVersion, should.Equal(int64(2)))
			assert.That(t, tj.Status, should.Equal(Status_PENDING))
		}
	})
	t.Run("Notify runs", func(t *testing.T) {
		rm := &rmMock{}
		m := NewMutator(rm)
		runID1 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("beef"))
		runID2 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("cafe"))
		tryjobIDs := make(common.TryjobIDs, 6)
		for i := range tryjobIDs {
			eid := MustBuildbucketID("example.bb.com", int64(6000+i))
			tj, err := m.Upsert(ctx, eid, func(tj *Tryjob) error {
				return nil
			})
			assert.That(t, err, should.ErrLike(nil))
			tryjobIDs[i] = tj.ID
		}
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			muts, err := m.BeginBatch(ctx, tryjobIDs)
			if err != nil {
				return err
			}
			for i, mut := range muts {
				if i%2 == 0 {
					mut.Tryjob.LaunchedBy = runID1
				} else {
					mut.Tryjob.LaunchedBy = runID2
				}
			}
			_, err = m.FinalizeBatch(ctx, muts)
			return err
		}, nil)
		assert.That(t, err, should.ErrLike(nil))

		expectedNotify := map[common.RunID]common.TryjobIDs{}
		for i, tjID := range tryjobIDs {
			if i%2 == 0 {
				expectedNotify[runID1] = append(expectedNotify[runID1], tjID)
			} else {
				expectedNotify[runID2] = append(expectedNotify[runID2], tjID)
			}
		}
		assert.That(t, rm.notified, should.Match(expectedNotify))
	})
}

type rmMock struct {
	notified   map[common.RunID]common.TryjobIDs
	notifiedMu sync.Mutex
}

func (rm *rmMock) NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, events *TryjobUpdatedEvents) error {
	rm.notifiedMu.Lock()
	defer rm.notifiedMu.Unlock()
	if rm.notified == nil {
		rm.notified = make(map[common.RunID]common.TryjobIDs)
	}
	for _, event := range events.Events {
		rm.notified[runID] = append(rm.notified[runID], common.TryjobID(event.TryjobId))
	}
	return nil
}
