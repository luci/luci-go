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

package bbfacade

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(tryjob.Tryjob{}))
}

func TestSearch(t *testing.T) {
	ftt.Run("Search", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		f := &Facade{
			ClientFactory: ct.BuildbucketFake.NewClientFactory(),
		}

		const (
			clid             = common.CLID(123)
			gHost            = "example-review.googlesource.com"
			gRepo            = "repo/example"
			gChangeNum       = 753
			gPatchset        = 10
			gMinEquiPatchset = 5

			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		equiBuilderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testEquivalentBuilder",
		}
		gc := &bbpb.GerritChange{
			Host:     gHost,
			Project:  gRepo,
			Change:   gChangeNum,
			Patchset: gPatchset,
		}
		epoch := ct.Clock.Now().UTC()
		cl := &run.RunCL{
			ID:         clid,
			ExternalID: changelist.MustGobID(gHost, gChangeNum),
			Detail: &changelist.Snapshot{
				Patchset:              gPatchset,
				MinEquivalentPatchset: gMinEquiPatchset,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: &gerritpb.ChangeInfo{
							Project: gRepo,
							Number:  gChangeNum,
						},
					},
				},
			},
		}
		definition := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host:    bbHost,
					Builder: builderID,
				},
			},
			EquivalentTo: &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host:    bbHost,
						Builder: equiBuilderID,
					},
				},
			},
		}

		ct.BuildbucketFake.AddBuilder(bbHost, builderID, nil)
		ct.BuildbucketFake.AddBuilder(bbHost, equiBuilderID, nil)
		bbClient := ct.BuildbucketFake.MustNewClient(ctx, bbHost, lProject)
		commonMutateFn := func(build *bbpb.Build) {
			build.Status = bbpb.Status_SUCCESS
			build.StartTime = timestamppb.New(epoch.Add(1 * time.Minute))
			build.EndTime = timestamppb.New(epoch.Add(2 * time.Minute))
		}

		t.Run("Single Buildbucket host", func(t *ftt.Test) {
			searchAll := func() []*tryjob.Tryjob {
				var ret []*tryjob.Tryjob
				err := f.Search(ctx, []*run.RunCL{cl}, []*tryjob.Definition{definition}, lProject, func(job *tryjob.Tryjob) bool {
					ret = append(ret, job)
					return true
				})
				assert.Loosely(t, err, should.BeNil)
				return ret
			}
			t.Run("Match", func(t *ftt.Test) {
				var build *bbpb.Build

				checkResult := func(t testing.TB) {
					t.Helper()

					results := searchAll()
					assert.Loosely(t, results, should.HaveLength(1), truth.LineContext())
					tj := results[0]
					assert.Loosely(t, tj.Result, should.NotBeNil, truth.LineContext())
					tj.Result = nil
					assert.Loosely(t, tj, should.Match(&tryjob.Tryjob{
						ExternalID: tryjob.MustBuildbucketID(bbHost, build.GetId()),
						Definition: definition,
						Status:     tryjob.Status_ENDED,
					}), truth.LineContext())
				}

				t.Run("Simple", func(t *ftt.Test) {
					var err error
					build, err = bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       builderID,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					checkResult(t)
				})

				t.Run("With permitted additional properties", func(t *ftt.Test) {
					prop, err := structpb.NewStruct(map[string]any{
						"$recipe_engine/cq": map[string]any{
							"active":   true,
							"run_mode": "FULL_RUN",
						},
					})
					assert.Loosely(t, err, should.BeNil)
					build, err = bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       builderID,
						Properties:    prop,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					checkResult(t)
				})

				t.Run("Match equivalent tryjob", func(t *ftt.Test) {
					var err error
					build, err = bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       equiBuilderID,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					checkResult(t)
				})

			})

			t.Run("No match", func(t *ftt.Test) {
				t.Run("Patchset out of range ", func(t *ftt.Test) {
					for _, ps := range []int{3, 11, 20} {
						assert.Loosely(t, ps < gMinEquiPatchset || ps > gPatchset, should.BeTrue)
						gc.Patchset = int64(ps)
						build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
							Builder:       builderID,
							GerritChanges: []*bbpb.GerritChange{gc},
						})
						assert.Loosely(t, err, should.BeNil)
						ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
						results := searchAll()
						assert.Loosely(t, results, should.BeEmpty)
					}
				})

				t.Run("Mismatch CL", func(t *ftt.Test) {
					anotherChange := proto.Clone(gc).(*bbpb.GerritChange)
					anotherChange.Change = anotherChange.Change + 50
					build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       builderID,
						GerritChanges: []*bbpb.GerritChange{anotherChange},
					})
					assert.Loosely(t, err, should.BeNil)
					ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					results := searchAll()
					assert.Loosely(t, results, should.BeEmpty)
				})

				t.Run("Mismatch Builder", func(t *ftt.Test) {
					anotherBuilder := &bbpb.BuilderID{
						Project: lProject,
						Bucket:  "anotherBucket",
						Builder: "anotherBuilder",
					}
					ct.BuildbucketFake.AddBuilder(bbHost, anotherBuilder, nil)
					build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       anotherBuilder,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					results := searchAll()
					assert.Loosely(t, results, should.BeEmpty)
				})

				t.Run("Not permitted additional properties", func(t *ftt.Test) {
					prop, err := structpb.NewStruct(map[string]any{
						"$recipe_engine/cq": map[string]any{
							"active":   true,
							"run_mode": "FULL_RUN",
						}, // permitted
						"foo": "bar", // not permitted
					})
					assert.Loosely(t, err, should.BeNil)
					build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       builderID,
						Properties:    prop,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
					results := searchAll()
					assert.Loosely(t, results, should.BeEmpty)
				})

				t.Run("Multiple CLs", func(t *ftt.Test) {
					t.Run("Build involves extra Gerrit change", func(t *ftt.Test) {
						anotherChange := proto.Clone(gc).(*bbpb.GerritChange)
						anotherChange.Change = anotherChange.Change + 1
						build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
							Builder:       builderID,
							GerritChanges: []*bbpb.GerritChange{gc, anotherChange},
						})
						assert.Loosely(t, err, should.BeNil)
						ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)
						results := searchAll()
						assert.Loosely(t, results, should.BeEmpty)
					})

					t.Run("Expecting extra Gerrit change", func(t *ftt.Test) {
						build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
							Builder:       builderID,
							GerritChanges: []*bbpb.GerritChange{gc},
						})
						assert.Loosely(t, err, should.BeNil)
						ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), commonMutateFn)

						anotherChange := proto.Clone(gc).(*bbpb.GerritChange)
						anotherChange.Change = anotherChange.Change + 1
						anotherCL := &run.RunCL{
							ID:         clid + 1,
							ExternalID: changelist.MustGobID(gHost, gChangeNum+1),
							Detail: &changelist.Snapshot{
								Patchset:              3,
								MinEquivalentPatchset: 1,
								Kind: &changelist.Snapshot_Gerrit{
									Gerrit: &changelist.Gerrit{
										Host: gHost,
										Info: &gerritpb.ChangeInfo{
											Project: gRepo,
											Number:  gChangeNum + 1,
										},
									},
								},
							},
						}
						var tryjobs []*tryjob.Tryjob
						err = f.Search(ctx, []*run.RunCL{cl, anotherCL}, []*tryjob.Definition{definition}, lProject, func(job *tryjob.Tryjob) bool {
							tryjobs = append(tryjobs, job)
							return true
						})
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, tryjobs, should.BeEmpty)
					})
				})
			})
		})

		t.Run("Paging builds", func(t *ftt.Test) {
			// Scenario:
			//  Buildbucket hosts defined in `bbHosts`. Each Buildbucket host has
			//  `numBuildsPerHost` of builds with build ID 1..numBuildsPerHost.
			//  Each even buildID is from builderFoo and each odd buildID is from
			//  builderBar
			bbHosts := []string{"bb-dev.example.com", "bb-staging.example.com", "bb-prod.example.com"}
			numBuildsPerHost := 50
			builderFoo := &bbpb.BuilderID{
				Project: lProject,
				Bucket:  "testBucket",
				Builder: "foo",
			}
			builderBar := &bbpb.BuilderID{
				Project: lProject,
				Bucket:  "testBucket",
				Builder: "bar",
			}
			allBuilds := make([]*bbpb.Build, 0, len(bbHosts)*numBuildsPerHost)
			for _, bbHost := range bbHosts {
				ct.BuildbucketFake.AddBuilder(bbHost, builderFoo, nil)
				ct.BuildbucketFake.AddBuilder(bbHost, builderBar, nil)
				bbClient = ct.BuildbucketFake.MustNewClient(ctx, bbHost, lProject)
				for i := 1; i <= numBuildsPerHost; i++ {
					epoch = ct.Clock.Now().UTC()
					builder := builderFoo
					if i%2 == 1 {
						builder = builderBar
					}
					build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder:       builder,
						GerritChanges: []*bbpb.GerritChange{gc},
					})
					assert.Loosely(t, err, should.BeNil)
					build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), func(build *bbpb.Build) {
						build.Status = bbpb.Status_SUCCESS
						build.StartTime = timestamppb.New(epoch.Add(1 * time.Minute))
						build.EndTime = timestamppb.New(epoch.Add(2 * time.Minute))
					})
					allBuilds = append(allBuilds, build)
					ct.Clock.Add(1 * time.Minute)
				}
			}
			t.Run("Search for builds from builderFoo", func(t *ftt.Test) {
				var definitions []*tryjob.Definition
				expected := stringset.New(numBuildsPerHost / 2 * len(bbHosts))
				for _, build := range allBuilds {
					if proto.Equal(build.GetBuilder(), builderFoo) {
						expected.Add(string(tryjob.MustBuildbucketID(build.GetInfra().GetBuildbucket().GetHostname(), build.GetId())))
					}
				}
				for _, bbHost := range bbHosts {
					definitions = append(definitions, &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host:    bbHost,
								Builder: builderFoo,
							},
						},
					})
				}
				got := stringset.New(numBuildsPerHost / 2 * len(bbHosts))
				err := f.Search(ctx, []*run.RunCL{cl}, definitions, lProject, func(job *tryjob.Tryjob) bool {
					assert.Loosely(t, got.Has(string(job.ExternalID)), should.BeFalse)
					got.Add(string(job.ExternalID))
					return true
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Resemble(expected))
			})

			t.Run("Can stop paging", func(t *ftt.Test) {
				var definitions []*tryjob.Definition
				for _, bbHost := range bbHosts {
					// matching all
					definitions = append(definitions,
						&tryjob.Definition{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host:    bbHost,
									Builder: builderFoo,
								},
							},
						},
						&tryjob.Definition{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host:    bbHost,
									Builder: builderBar,
								},
							},
						},
					)
				}
				stopAfter := numBuildsPerHost * len(bbHosts) / 2
				count := 0

				err := f.Search(ctx, []*run.RunCL{cl}, definitions, lProject, func(job *tryjob.Tryjob) bool {
					count++
					switch {
					case count < stopAfter:
						return true
					case count == stopAfter:
						return false
					default:
						assert.Loosely(t, "Callback is called after it indicates to stop", should.BeEmpty)
						return true // never reached
					}
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}
