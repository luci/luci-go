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
	"go.chromium.org/luci/gae/service/datastore"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestWorker(t *testing.T) {
	t.Parallel()

	Convey("Start", t, func() {
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
			rm: run.NewNotifier(ct.TQDispatcher),
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
			tj := w.makeBaseTryjob(ctx)
			tj.ExternalID = eid
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
			So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
			}, nil), ShouldBeNil)
			return tj
		}
		Convey("Reuse", func() {
			eid := tryjob.MustBuildbucketID(bbHost, mathrand.Int63(ctx))
			w.findReuseFns = []findReuseFn{
				func(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					So(definitions, ShouldHaveLength, 1)
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: makeReuseTryjob(ctx, def, eid),
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ExternalID, ShouldEqual, eid)
			So(w.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
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
			})
		})

		Convey("No reuse, launch new tryjob", func() {
			w.findReuseFns = []findReuseFn{
				func(context.Context, []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					return nil, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ExternalID, ShouldNotBeEmpty)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_TRIGGERED)
		})

		Convey("Reuse find previous PENDING tryjob", func() {
			var reuseID common.TryjobID
			w.findReuseFns = []findReuseFn{
				func(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					So(definitions, ShouldHaveLength, 1)
					tj := w.makePendingTryjob(ctx, definitions[0])
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
					}, nil), ShouldBeNil)
					reuseID = tj.ID
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: tj,
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, []*tryjob.Definition{def})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ID, ShouldEqual, reuseID)
			So(tryjobs[0].ExternalID, ShouldNotBeEmpty)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_TRIGGERED)
		})

		Convey("Mix", func() {
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
					So(defs, ShouldResembleProto, definitions)
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[0]: makeReuseTryjob(ctx, def, eidForBuilder0),
					}, nil
				},
				func(ctx context.Context, defs []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
					// reuse Tryjob already found for definitions[0]
					So(defs, ShouldResembleProto, definitions[1:])
					return map[*tryjob.Definition]*tryjob.Tryjob{
						definitions[2]: makeReuseTryjob(ctx, def, eidForBuilder2),
					}, nil
				},
			}
			tryjobs, err := w.start(ctx, definitions)
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 3)
			So(tryjobs[0].ExternalID, ShouldEqual, eidForBuilder0)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_ENDED)
			So(tryjobs[1].ExternalID, ShouldNotBeEmpty)
			So(tryjobs[1].Status, ShouldEqual, tryjob.Status_TRIGGERED)
			So(tryjobs[2].ExternalID, ShouldEqual, eidForBuilder2)
			So(tryjobs[2].Status, ShouldEqual, tryjob.Status_ENDED)
		})
	})
}
