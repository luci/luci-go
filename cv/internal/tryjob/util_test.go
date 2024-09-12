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

package tryjob

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestSaveTryjobs(t *testing.T) {
	t.Parallel()

	ftt.Run("SaveTryjobs works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const bbHost = "buildbucket.example.com"
		now := ct.Clock.Now().UTC()
		var runID = common.MakeRunID("infra", now.Add(-1*time.Hour), 1, []byte("foo"))
		var notified map[common.RunID]common.TryjobIDs
		notifyFn := func(ctx context.Context, runID common.RunID, events *TryjobUpdatedEvents) error {
			if notified == nil {
				notified = make(map[common.RunID]common.TryjobIDs)
			}
			for _, evt := range events.Events {
				notified[runID] = append(notified[runID], common.TryjobID(evt.TryjobId))
			}
			return nil
		}

		t.Run("No internal ID", func(t *ftt.Test) {
			t.Run("No external ID", func(t *ftt.Test) {
				tj := &Tryjob{
					EVersion:         1,
					EntityCreateTime: now,
					EntityUpdateTime: now,
					LaunchedBy:       runID,
				}
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
				}, nil), should.BeNil)
				assert.Loosely(t, tj.ID, should.NotEqual(0))
				assert.Loosely(t, notified, should.Resemble(map[common.RunID]common.TryjobIDs{
					runID: {tj.ID},
				}))
			})
			t.Run("External ID provided", func(t *ftt.Test) {
				t.Run("Does not map to any existing tryjob", func(t *ftt.Test) {
					eid := MustBuildbucketID(bbHost, 10)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
						LaunchedBy:       runID,
					}
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), should.BeNil)
					tj, err := eid.Load(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tj, should.NotBeNil)
					assert.Loosely(t, notified, should.Resemble(map[common.RunID]common.TryjobIDs{
						runID: {tj.ID},
					}))
				})
				t.Run("Map to an existing tryjob", func(t *ftt.Test) {
					eid := MustBuildbucketID(bbHost, 10)
					eid.MustCreateIfNotExists(ctx)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
						LaunchedBy:       runID,
					}
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), should.ErrLike("has already mapped to internal id"))
					assert.Loosely(t, notified, should.BeEmpty)
				})
			})
		})

		t.Run("Internal ID provided", func(t *ftt.Test) {
			const tjID = common.TryjobID(45366)
			t.Run("No external ID", func(t *ftt.Test) {
				tj := &Tryjob{
					ID:               tjID,
					EVersion:         1,
					EntityCreateTime: now,
					EntityUpdateTime: now,
					ReusedBy:         common.RunIDs{runID},
				}
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
				}, nil), should.BeNil)
				assert.Loosely(t, notified, should.Resemble(map[common.RunID]common.TryjobIDs{
					runID: {tjID},
				}))
			})
			t.Run("External ID provided", func(t *ftt.Test) {
				t.Run("Does not map to any existing tryjob", func(t *ftt.Test) {
					eid := MustBuildbucketID(bbHost, 10)
					tj := &Tryjob{
						ID:               tjID,
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
						ReusedBy:         common.RunIDs{runID},
					}
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), should.BeNil)
					assert.Loosely(t, eid.MustLoad(ctx).ID, should.Equal(tjID))
					assert.Loosely(t, notified, should.Resemble(map[common.RunID]common.TryjobIDs{
						runID: {tjID},
					}))
				})

				t.Run("Map to an existing tryjob", func(t *ftt.Test) {
					t.Run("existing tryjob has the same ID", func(t *ftt.Test) {
						eid := MustBuildbucketID(bbHost, 10)
						existing := eid.MustCreateIfNotExists(ctx)
						ct.Clock.Add(10 * time.Minute)
						tj := &Tryjob{
							ID:               existing.ID,
							ExternalID:       eid,
							EVersion:         existing.EVersion + 1,
							EntityUpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
							ReusedBy:         common.RunIDs{runID},
						}
						assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
						}, nil), should.BeNil)
						assert.Loosely(t, eid.MustLoad(ctx).EVersion, should.Equal(existing.EVersion+1))
						assert.Loosely(t, notified, should.Resemble(map[common.RunID]common.TryjobIDs{
							runID: {tj.ID},
						}))
					})
					t.Run("existing tryjob has a different ID", func(t *ftt.Test) {
						eid := MustBuildbucketID(bbHost, 10)
						existing := eid.MustCreateIfNotExists(ctx)
						ct.Clock.Add(10 * time.Minute)
						tj := &Tryjob{
							ID:               existing.ID + 10,
							ExternalID:       eid,
							EVersion:         existing.EVersion + 1,
							EntityUpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
							ReusedBy:         common.RunIDs{runID},
						}
						assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
						}, nil), should.ErrLike("has already mapped to"))
						assert.Loosely(t, notified, should.BeEmpty)
					})
				})
			})
		})
	})
}

func TestQueryTryjobIDsUpdatedBefore(t *testing.T) {
	t.Parallel()

	ftt.Run("QueryTryjobIDsUpdatedBefore", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		nextBuildID := int64(1)
		createNTryjobs := func(n int) []*Tryjob {
			tryjobs := make([]*Tryjob, n)
			for i := range tryjobs {
				eid := MustBuildbucketID("example.com", nextBuildID)
				nextBuildID++
				tryjobs[i] = eid.MustCreateIfNotExists(ctx)
			}
			return tryjobs
		}

		var allTryjobs []*Tryjob
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)

		before := ct.Clock.Now().Add(-30 * time.Second)
		var expected common.TryjobIDs
		for _, tj := range allTryjobs {
			if tj.EntityUpdateTime.Before(before) {
				expected = append(expected, tj.ID)
			}
		}
		slices.Sort(expected)

		actual, err := QueryTryjobIDsUpdatedBefore(ctx, before)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(expected))
	})
}
