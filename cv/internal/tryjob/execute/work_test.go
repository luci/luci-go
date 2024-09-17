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
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/rand/mathrand"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestWorker(t *testing.T) {
	t.Parallel()

	ftt.Run("Start", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject     = "testProj"
			bbHost       = "buildbucket.example.com"
			buildID      = 9524107902457
			reuseKey     = "cafecafe"
			clid         = 34586452134
			gHost        = "example-review.com"
			gRepo        = "repo/a"
			gChange      = 123
			gPatchset    = 5
			gMinPatchset = 4
		)
		var runID = common.MakeRunID(lProject, ct.Clock.Now().Add(-1*time.Hour), 1, []byte("abcd"))

		w := &worker{
			run: &run.Run{
				ID:   runID,
				Mode: run.DryRun,
				CLs:  common.CLIDs{clid},
			},
			cls: []*run.RunCL{
				{
					ID: clid,
					Detail: &changelist.Snapshot{
						Patchset:              gPatchset,
						MinEquivalentPatchset: gMinPatchset,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: &gerritpb.ChangeInfo{
									Project: gRepo,
									Number:  gChange,
									Owner: &gerritpb.AccountInfo{
										Email: "owner@example.com",
									},
								},
							},
						},
					},
					Trigger: &run.Trigger{
						Mode:  string(run.DryRun),
						Email: "triggerer@example.com",
					},
				},
			},
			knownTryjobIDs: make(common.TryjobIDSet),
			reuseKey:       reuseKey,
			clPatchsets:    tryjob.CLPatchsets{tryjob.MakeCLPatchset(clid, gPatchset)},
			backend: &bbfacade.Facade{
				ClientFactory: ct.BuildbucketFake.NewClientFactory(),
			},
			mutator: tryjob.NewMutator(run.NewNotifier(ct.TQDispatcher)),
		}
		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "someBucket",
			Builder: "someBuilder",
		}
		def := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host:    bbHost,
					Builder: builder,
				},
			},
		}
		ct.BuildbucketFake.AddBuilder(bbHost, builder, nil)

		makeReuseTryjob := func(ctx context.Context, def *tryjob.Definition, eid tryjob.ExternalID) *tryjob.Tryjob {
			t.Helper()
			tj, err := w.mutator.Upsert(ctx, eid, func(tj *tryjob.Tryjob) error {
				tj.Definition = def
				tj.Status = tryjob.Status_ENDED
				tj.ReusedBy = append(tj.ReusedBy, w.run.ID)
				tj.Result = &tryjob.Result{
					CreateTime: timestamppb.New(ct.Clock.Now().UTC().Add(-staleTryjobAge / 2)),
					Backend: &tryjob.Result_Buildbucket_{
						Buildbucket: &tryjob.Result_Buildbucket{
							Builder: builder,
						},
					},
					Status: tryjob.Result_SUCCEEDED,
				}
				tj.CLPatchsets = w.clPatchsets
				tj.ReuseKey = w.reuseKey
				return nil
			})
			assert.That(t, err, should.ErrLike(nil), truth.LineContext())
			return tj
		}
		t.Run("Reuse", func(t *ftt.Test) {
			eid := tryjob.MustBuildbucketID(bbHost, mathrand.Int63(ctx))
			w.findReuseFns = []findReuseFn{
				func(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					assert.Loosely(t, definitions, should.HaveLength(1))
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: makeReuseTryjob(ctx, def, eid),
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			assert.Loosely(t, tryjobs[0].ExternalID, should.Equal(eid))
			assert.Loosely(t, w.logEntries, should.Resemble([]*tryjob.ExecutionLogEntry{
				{
					Time: timestamppb.New(ct.Clock.Now().UTC()),
					Kind: &tryjob.ExecutionLogEntry_TryjobsReused_{
						TryjobsReused: &tryjob.ExecutionLogEntry_TryjobsReused{
							Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
								makeLogTryjobSnapshot(def, tryjobs[0], true),
							},
						},
					},
				},
			}))
		})

		t.Run("No reuse, launch new tryjob", func(t *ftt.Test) {
			w.findReuseFns = []findReuseFn{
				func(context.Context, []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					return nil, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			assert.Loosely(t, tryjobs[0].ExternalID, should.NotBeEmpty)
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_TRIGGERED))
		})

		t.Run("Reuse find previous PENDING tryjob", func(t *ftt.Test) {
			var reuseID common.TryjobID
			w.findReuseFns = []findReuseFn{
				func(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					assert.Loosely(t, definitions, should.HaveLength(1))
					tj := w.makePendingTryjob(ctx, definitions[0])
					assert.That(t, datastore.Put(ctx, tj), should.ErrLike(nil))
					reuseID = tj.ID
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: tj,
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			assert.Loosely(t, tryjobs[0].ID, should.Equal(reuseID))
			assert.Loosely(t, tryjobs[0].ExternalID, should.NotBeEmpty)
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_TRIGGERED))
		})

		t.Run("Mix", func(t *ftt.Test) {
			baseDef := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: bbHost,
						Builder: &bbpb.BuilderID{
							Project: lProject,
							Bucket:  "some_bucket",
							Builder: "builder",
						},
					},
				},
			}

			// Initialize Definitions of 3 builders.
			definitions := make([]*tryjob.Definition, 3)
			for i := range definitions {
				definitions[i] = proto.Clone(baseDef).(*tryjob.Definition)
				definitions[i].GetBuildbucket().GetBuilder().Builder += fmt.Sprintf("-%d", i)
				ct.BuildbucketFake.AddBuilder(bbHost, definitions[i].GetBuildbucket().GetBuilder(), nil)
			}

			// Define two reuse functions. The first function will find reuse Tryjob
			// for the first builder. The second function will find reuse Tryjob for
			// the third builder.
			eidForBuilder0 := tryjob.MustBuildbucketID(bbHost, mathrand.Int63(ctx))
			eidForBuilder2 := tryjob.MustBuildbucketID(bbHost, mathrand.Int63(ctx))
			w.findReuseFns = []findReuseFn{
				func(ctx context.Context, defs []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					assert.Loosely(t, defs, should.Resemble(definitions))
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: makeReuseTryjob(ctx, def, eidForBuilder0),
					}, nil
				},
				func(ctx context.Context, defs []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					// reuse Tryjob already found for definitions[0]
					assert.Loosely(t, defs, should.Resemble(definitions[1:]))
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[2]: makeReuseTryjob(ctx, def, eidForBuilder2),
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, definitions)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tryjobs, should.HaveLength(3))
			assert.Loosely(t, tryjobs[0].ExternalID, should.Equal(eidForBuilder0))
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_ENDED))
			assert.Loosely(t, tryjobs[1].ExternalID, should.NotBeEmpty)
			assert.Loosely(t, tryjobs[1].Status, should.Equal(tryjob.Status_TRIGGERED))
			assert.Loosely(t, tryjobs[2].ExternalID, should.Equal(eidForBuilder2))
			assert.Loosely(t, tryjobs[2].Status, should.Equal(tryjob.Status_ENDED))
		})
	})
}
