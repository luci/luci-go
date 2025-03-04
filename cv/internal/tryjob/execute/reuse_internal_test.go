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

package execute

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestFindReuseInCV(t *testing.T) {
	t.Parallel()

	ftt.Run("FindReuseInCV", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const reuseKey = "cafecafe"
		const lProject = "testProj"
		const tjID = 6436
		var runID = common.MakeRunID(lProject, ct.Clock.Now().Add(-1*time.Hour), 1, []byte("abcd"))
		w := &worker{
			run: &run.Run{
				ID:   runID,
				Mode: run.DryRun,
			},
			knownTryjobIDs: make(common.TryjobIDSet),
			mutator:        tryjob.NewMutator(run.NewNotifier(ct.TQDispatcher)),
		}
		builder := &bbpb.BuilderID{
			Project: "ProjectFoo",
			Bucket:  "BucketFoo",
			Builder: "BuilderFoo",
		}
		defFoo := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host:    "buildbucket.example.com",
					Builder: builder,
				},
			},
		}

		now := ct.Clock.Now().UTC()
		tj := &tryjob.Tryjob{
			ID:               tjID,
			ExternalID:       tryjob.MustBuildbucketID("buildbucket.example.com", 9821),
			EVersion:         2,
			EntityCreateTime: now.Add(-staleTryjobAge / 2),
			EntityUpdateTime: now.Add(-1 * time.Minute),
			ReuseKey:         reuseKey,
			Definition:       defFoo,
			Status:           tryjob.Status_ENDED,
			LaunchedBy:       common.MakeRunID(lProject, now.Add(-2*time.Hour), 1, []byte("efgh")),
			Result: &tryjob.Result{
				CreateTime: timestamppb.New(now.Add(-staleTryjobAge / 2)),
				Backend: &tryjob.Result_Buildbucket_{
					Buildbucket: &tryjob.Result_Buildbucket{
						Builder: builder,
					},
				},
				Status: tryjob.Result_SUCCEEDED,
			},
		}

		t.Run("No tryjob to reuse", func(t *ftt.Test) {
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Found Reuse", func(t *ftt.Test) {
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].EVersion, should.Equal(tj.EVersion+1))
			assert.That(t, result[defFoo].EntityUpdateTime, should.Match(now))
			assert.That(t, result[defFoo].ReusedBy, should.Match(common.RunIDs{runID}))
		})

		t.Run("Definition doesn't match but builder matches", func(t *ftt.Test) {
			tj.Definition = &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
						Builder: &bbpb.BuilderID{
							Project: "ProjectBar",
							Bucket:  "BucketBar",
							Builder: "BuilderBar",
						},
					},
				},
				EquivalentTo: defFoo,
			}
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
		})

		t.Run("None of definition and builder matches", func(t *ftt.Test) {
			tj.Definition = &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
						Builder: &bbpb.BuilderID{
							Project: "ProjectBar",
							Bucket:  "BucketBar",
							Builder: "BuilderBar",
						},
					},
				},
			}
			tj.Result.GetBuildbucket().Builder = &bbpb.BuilderID{
				Project: "ProjectBar",
				Bucket:  "BucketBar",
				Builder: "BuilderBar",
			}
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Tryjob already known", func(t *ftt.Test) {
			w.knownTryjobIDs.Add(tjID)
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Tryjob is from different project", func(t *ftt.Test) {
			tj.LaunchedBy = common.MakeRunID("anotherProj", now.Add(-2*time.Hour), 1, []byte("cool"))
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Tryjob is not reusable", func(t *ftt.Test) {
			tj.Result.CreateTime = timestamppb.New(now.Add(-2 * staleTryjobAge))
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Pick the latest one", func(t *ftt.Test) {
			newerTryjob := *tj // shallow copy
			newerTryjob.ID = tjID - 1
			newerTryjob.EntityCreateTime = tj.EntityCreateTime.Add(1 * time.Minute)
			assert.NoErr(t, datastore.Put(ctx, tj, &newerTryjob))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].ID, should.Equal(newerTryjob.ID))
			assert.That(t, result[defFoo].ReusedBy, should.Match(common.RunIDs{runID}))
			assert.NoErr(t, datastore.Get(ctx, tj))
			assert.Loosely(t, tj.ReusedBy.Index(runID), should.BeLessThan(0))
		})

		t.Run("Run already in Tryjob", func(t *ftt.Test) {
			tj.ReusedBy = common.RunIDs{runID}
			assert.NoErr(t, datastore.Put(ctx, tj))
			result, err := w.findReuseInCV(ctx, reuseKey, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].ID, should.Equal(tj.ID))
			assert.Loosely(t, result[defFoo].EVersion, should.Equal(tj.EVersion))
			assert.That(t, result[defFoo].EntityUpdateTime, should.Match(tj.EntityUpdateTime))
			assert.That(t, result[defFoo].ReusedBy, should.Match(common.RunIDs{runID}))
		})
	})
}
