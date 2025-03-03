// Copyright 2020 The LUCI Authors.
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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

func TestCL(t *testing.T) {
	t.Parallel()

	ftt.Run("CL", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		eid, err := GobID("x-review.example.com", 12)
		assert.NoErr(t, err)

		t.Run("ExternalID.Get returns nil if CL doesn't exist", func(t *ftt.Test) {
			cl, err := eid.Load(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, cl, should.BeNil)
		})

		t.Run("ExternalID.MustCreateIfNotExists creates a CL", func(t *ftt.Test) {
			cl := eid.MustCreateIfNotExists(ctx)
			assert.Loosely(t, cl, should.NotBeNil)
			assert.That(t, cl.ExternalID, should.Match(eid))
			// ID must be autoset to non-0 value.
			assert.Loosely(t, cl.ID, should.NotEqual(0))
			assert.Loosely(t, cl.EVersion, should.Equal(1))
			assert.That(t, cl.UpdateTime, should.Match(epoch))
			assert.Loosely(t, cl.RetentionKey, should.Equal(fmt.Sprintf("%02d/%010d", cl.ID%retentionKeyShards, epoch.Unix())))

			t.Run("ExternalID.Get loads existing CL", func(t *ftt.Test) {
				cl2, err := eid.Load(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, cl2.ID, should.Equal(cl.ID))
				assert.Loosely(t, cl2.ExternalID, should.Equal(eid))
				assert.Loosely(t, cl2.EVersion, should.Equal(1))
				assert.That(t, cl2.UpdateTime, should.Match(cl.UpdateTime))
				assert.That(t, cl2.Snapshot, should.Match(cl.Snapshot))
			})

			t.Run("ExternalID.MustCreateIfNotExists loads existing CL", func(t *ftt.Test) {
				cl3 := eid.MustCreateIfNotExists(ctx)
				assert.Loosely(t, cl3, should.NotBeNil)
				assert.Loosely(t, cl3.ID, should.Equal(cl.ID))
				assert.That(t, cl3.ExternalID, should.Match(eid))
				assert.Loosely(t, cl3.EVersion, should.Equal(1))
				assert.That(t, cl3.UpdateTime, should.Match(cl.UpdateTime))
				assert.That(t, cl3.Snapshot, should.Match(cl.Snapshot))
			})

			t.Run("Delete works", func(t *ftt.Test) {
				err := Delete(ctx, cl.ID)
				assert.NoErr(t, err)
				// Verify.
				assert.That(t, datastore.Get(ctx, cl), should.ErrLikeError(datastore.ErrNoSuchEntity))
				cl2, err2 := eid.Load(ctx)
				assert.Loosely(t, err2, should.BeNil)
				assert.Loosely(t, cl2, should.BeNil)

				t.Run("delete is now a noop", func(t *ftt.Test) {
					err := Delete(ctx, cl.ID)
					assert.NoErr(t, err)
				})
			})
		})
	})
}

func TestLookup(t *testing.T) {
	t.Parallel()

	ftt.Run("Lookup works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		const n = 10
		ids := make([]common.CLID, n)
		eids := make([]ExternalID, n)
		for i := range eids {
			eids[i] = MustGobID("x-review.example.com", int64(i+1))
			if i%2 == 0 {
				ids[i] = eids[i].MustCreateIfNotExists(ctx).ID
			}
		}

		actual, err := Lookup(ctx, eids)
		assert.NoErr(t, err)
		assert.That(t, actual, should.Match(ids))
	})
}

func makeSnapshot(luciProject string, updatedTime time.Time) *Snapshot {
	return &Snapshot{
		ExternalUpdateTime: timestamppb.New(updatedTime),
		Kind: &Snapshot_Gerrit{Gerrit: &Gerrit{
			Info: &gerritpb.ChangeInfo{
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number: 1,
						Kind:   gerritpb.RevisionInfo_REWORK,
					},
				},
			},
		}},
		MinEquivalentPatchset: 1,
		Patchset:              2,
		LuciProject:           luciProject,
	}
}
