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

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	bbfake "go.chromium.org/luci/cv/internal/buildbucket/fake"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearch(t *testing.T) {
	Convey("Search", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
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

			bbHost  = "buildbucket.example.com"
			buildID = 5555

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

		templateBuild := bbfake.NewBuildConstructor().
			WithID(buildID).
			WithHost(bbHost).
			WithBuilderID(builderID).
			WithStatus(bbpb.Status_SUCCESS).
			WithCreateTime(epoch).
			WithStartTime(epoch.Add(1 * time.Minute)).
			WithEndTime(epoch.Add(2 * time.Minute)).
			AppendGerritChanges(gc).
			Construct()

		Convey("Single Buildbucket host", func() {
			searchAll := func() []*tryjob.Tryjob {
				var ret []*tryjob.Tryjob
				err := f.Search(ctx, []*run.RunCL{cl}, []*tryjob.Definition{definition}, lProject, func(t *tryjob.Tryjob) bool {
					ret = append(ret, t)
					return true
				})
				So(err, ShouldBeNil)
				return ret
			}
			Convey("Match", func() {
				var build *bbpb.Build
				Convey("Simple", func() {
					build = bbfake.NewConstructorFromBuild(templateBuild).Construct()
				})
				Convey("With premitted additional properties", func() {
					build = bbfake.NewConstructorFromBuild(templateBuild).
						WithRequestedProperties(map[string]interface{}{
							"$recipe_engine/cq": map[string]interface{}{
								"active":   true,
								"run_mode": "FULL_RUN",
							},
						}).
						Construct()
				})
				Convey("Match equivalent tryjob", func() {
					build = bbfake.NewConstructorFromBuild(templateBuild).
						WithBuilderID(equiBuilderID).
						Construct()
				})
				ct.BuildbucketFake.AddBuild(build)
				results := searchAll()
				So(results, ShouldHaveLength, 1)
				tj := results[0]
				So(tj.Result, ShouldNotBeNil)
				tj.Result = nil
				So(tj, cvtesting.SafeShouldResemble, &tryjob.Tryjob{
					ExternalID: tryjob.MustBuildbucketID(bbHost, buildID),
					Definition: definition,
					Status:     tryjob.Status_ENDED,
				})
			})

			Convey("No match", func() {
				Convey("Patchset out of range ", func() {
					for _, ps := range []int{3, 11, 20} {
						So(ps < gMinEquiPatchset || ps > gPatchset, ShouldBeTrue)
						gc.Patchset = int64(ps)
						build := bbfake.NewConstructorFromBuild(templateBuild).
							ResetGerritChanges().
							AppendGerritChanges(gc).
							Construct()
						ct.BuildbucketFake.AddBuild(build)
						results := searchAll()
						So(results, ShouldBeEmpty)
					}
				})

				Convey("Mismatch CL", func() {
					anotherChange := proto.Clone(gc).(*bbpb.GerritChange)
					anotherChange.Change = anotherChange.Change + 50
					build := bbfake.NewConstructorFromBuild(templateBuild).
						ResetGerritChanges().
						AppendGerritChanges(anotherChange).
						Construct()
					ct.BuildbucketFake.AddBuild(build)
					results := searchAll()
					So(results, ShouldBeEmpty)
				})

				Convey("Mismatch Builder", func() {
					build := bbfake.NewConstructorFromBuild(templateBuild).
						WithBuilderID(&bbpb.BuilderID{
							Project: lProject,
							Bucket:  "anotherBucket",
							Builder: "anotherBuilder",
						}).
						Construct()
					ct.BuildbucketFake.AddBuild(build)
					results := searchAll()
					So(results, ShouldBeEmpty)
				})

				Convey("Not premitted additional properties", func() {
					build := bbfake.NewConstructorFromBuild(templateBuild).
						WithRequestedProperties(map[string]interface{}{
							"$recipe_engine/cq": map[string]interface{}{
								"active":   true,
								"run_mode": "FULL_RUN",
							}, // premitted
							"foo": "bar", // not premitted
						}).
						Construct()
					ct.BuildbucketFake.AddBuild(build)
					results := searchAll()
					So(results, ShouldBeEmpty)
				})

				Convey("Multiple CLs", func() {
					Convey("Build involves extra Gerrit change", func() {
						anotherChange := proto.Clone(gc).(*bbpb.GerritChange)
						anotherChange.Change = anotherChange.Change + 1
						build := bbfake.NewConstructorFromBuild(templateBuild).
							AppendGerritChanges(gc, anotherChange).
							Construct()
						ct.BuildbucketFake.AddBuild(build)
						results := searchAll()
						So(results, ShouldBeEmpty)
					})

					Convey("Expecting extra Gerrit change", func() {
						build := bbfake.NewConstructorFromBuild(templateBuild).Construct()
						ct.BuildbucketFake.AddBuild(build)
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
						err := f.Search(ctx, []*run.RunCL{cl, anotherCL}, []*tryjob.Definition{definition}, lProject, func(t *tryjob.Tryjob) bool {
							tryjobs = append(tryjobs, t)
							return true
						})
						So(err, ShouldBeNil)
						So(tryjobs, ShouldBeEmpty)
					})
				})
			})
		})

		Convey("Paging builds", func() {
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
			for _, bbHost := range bbHosts {
				for buildID := 1; buildID <= numBuildsPerHost; buildID++ {
					builder := builderFoo
					if buildID%2 == 1 {
						builder = builderBar
					}
					build := bbfake.NewBuildConstructor().
						WithID(int64(buildID)).
						WithHost(bbHost).
						WithBuilderID(builder).
						WithStatus(bbpb.Status_SUCCESS).
						// newer build has smaller buildID
						WithCreateTime(epoch.Add(-time.Duration(buildID) * time.Minute)).
						WithStartTime(epoch.Add(-2 * time.Duration(buildID) * time.Minute)).
						WithEndTime(epoch.Add(-3 * time.Duration(buildID) * time.Minute)).
						AppendGerritChanges(gc).
						Construct()
					ct.BuildbucketFake.AddBuild(build)
				}
			}
			Convey("Search for builds from builderFoo", func() {
				var definitions []*tryjob.Definition
				expected := stringset.New(numBuildsPerHost / 2 * len(bbHosts))
				for _, bbHost := range bbHosts {
					definitions = append(definitions, &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host:    bbHost,
								Builder: builderFoo,
							},
						},
					})
					for buildID := 2; buildID <= numBuildsPerHost; buildID += 2 {
						expected.Add(string(tryjob.MustBuildbucketID(bbHost, int64(buildID))))
					}
				}
				got := stringset.New(numBuildsPerHost / 2 * len(bbHosts))
				err := f.Search(ctx, []*run.RunCL{cl}, definitions, lProject, func(t *tryjob.Tryjob) bool {
					So(got.Has(string(t.ExternalID)), ShouldBeFalse)
					got.Add(string(t.ExternalID))
					return true
				})
				So(err, ShouldBeNil)
				So(got, ShouldResemble, expected)
			})

			Convey("Can stop paging", func() {
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

				err := f.Search(ctx, []*run.RunCL{cl}, definitions, lProject, func(t *tryjob.Tryjob) bool {
					count++
					switch {
					case count < stopAfter:
						return true
					case count == stopAfter:
						return false
					default:
						So("Callback is called after it indicates to stop", ShouldBeEmpty)
						return true // never reached
					}
				})
				So(err, ShouldBeNil)
			})
		})
	})
}
