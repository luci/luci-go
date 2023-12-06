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

package requirement

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIsModeAllowed(t *testing.T) {
	Convey("isModeAllowed works", t, func() {
		Convey("when the mode is allowed", func() {
			So(isModeAllowed(run.DryRun, []string{string(run.FullRun), string(run.DryRun)}), ShouldBeTrue)
		})
		Convey("when the mode is not allowed", func() {
			So(isModeAllowed(run.DryRun, []string{string(run.FullRun)}), ShouldBeFalse)
		})
	})
}

func TestDefinitionMaker(t *testing.T) {
	Convey("definition maker works", t, func() {
		valid := "a/b/c"
		alternateValid := "a/b/x"
		invalidShort := "d/e"
		invalidLong := "f/g/h/i"

		b := &cfgpb.Verifiers_Tryjob_Builder{
			Name: valid,
			EquivalentTo: &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
				Name: alternateValid,
			},
			ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_UNSET,
		}

		Convey("main only", func() {
			Convey("flags off", func() {
				def := (&definitionMaker{
					builder:     b,
					equivalence: mainOnly,
					criticality: nonCritical,
				}).make()
				So(def, ShouldResembleProto, &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				})
			})
			Convey("flags on", func() {
				b.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				b.ExperimentPercentage = 49.9
				b.DisableReuse = true
				def := (&definitionMaker{
					builder:     b,
					equivalence: mainOnly,
					criticality: critical,
				}).make()
				So(def, ShouldResembleProto, &tryjob.Definition{
					DisableReuse:     true,
					Critical:         true,
					Optional:         true,
					ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED,
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				})
			})
		})
		Convey("equivalent only", func() {
			def := (&definitionMaker{
				builder:     b,
				equivalence: equivalentOnly,
				criticality: nonCritical,
			}).make()
			So(def, ShouldResembleProto, &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "cr-buildbucket.appspot.com",
						Builder: &buildbucketpb.BuilderID{
							Project: "a",
							Bucket:  "b",
							Builder: "x",
						},
					},
				},
			})
		})
		Convey("both", func() {
			def := (&definitionMaker{
				builder:     b,
				equivalence: bothMainAndEquivalent,
				criticality: nonCritical,
			}).make()
			So(def, ShouldResembleProto, &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "cr-buildbucket.appspot.com",
						Builder: &buildbucketpb.BuilderID{
							Project: "a",
							Bucket:  "b",
							Builder: "c",
						},
					},
				},
				EquivalentTo: &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "x",
							},
						},
					},
				},
			})
		})
		Convey("flipped", func() {
			def := (&definitionMaker{
				builder:     b,
				equivalence: flipMainAndEquivalent,
				criticality: nonCritical,
			}).make()
			So(def, ShouldResembleProto, &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "cr-buildbucket.appspot.com",
						Builder: &buildbucketpb.BuilderID{
							Project: "a",
							Bucket:  "b",
							Builder: "x",
						},
					},
				},
				EquivalentTo: &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				},
			})
		})
		Convey("empty buildername in main", func() {
			b.Name = ""
			dm := &definitionMaker{
				builder:     b,
				equivalence: mainOnly,
				criticality: critical,
			}
			So(func() { dm.make() }, ShouldPanicLike, "unexpectedly empty")
		})
		Convey("empty buildername in equivalent", func() {
			b.EquivalentTo.Name = ""
			dm := &definitionMaker{
				builder:     b,
				equivalence: equivalentOnly,
				criticality: critical,
			}
			So(func() { dm.make() }, ShouldPanicLike, "unexpectedly empty")
		})
		Convey("short buildername", func() {
			So(func() { makeBuildbucketDefinition(invalidShort) }, ShouldPanicLike, "unexpected format")
		})
		Convey("long buildername", func() {
			So(func() { makeBuildbucketDefinition(invalidLong) }, ShouldPanicLike, "unexpected format")
		})
	})
}

var (
	group1 = "group-one"
	userA  = identity.Identity("user:usera@example.com")
	userB  = identity.Identity("user:userb@example.com")
	group2 = "group-two"
	userD  = identity.Identity("user:userd@example.com")
)

func makeFakeAuthState(ctx context.Context) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		FakeDB: authtest.NewFakeDB(
			authtest.MockMembership(userA, group1),
			authtest.MockMembership(userB, group1),
			authtest.MockMembership(userD, group2),
		),
	})
}

func TestGetDisallowedOwners(t *testing.T) {
	ctx := makeFakeAuthState(context.Background())
	Convey("getDisallowedOwners", t, func() {
		Convey("works", func() {
			Convey("with no allowlists", func() {
				disallowed, err := getDisallowedOwners(ctx, []string{userA.Email()})
				So(err, ShouldBeNil)
				So(disallowed, ShouldHaveLength, 0)
			})
		})
		Convey("panics", func() {
			Convey("with nil users", func() {
				So(func() { _, _ = getDisallowedOwners(ctx, nil, group1) }, ShouldPanicLike, "nil user")
			})
			Convey("with zero users", func() {
				So(func() { _, _ = getDisallowedOwners(ctx, []string{}, group1) }, ShouldPanicLike, "nil user")
			})
		})
	})
}

func TestCompute(t *testing.T) {
	Convey("Compute works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		ctx = makeFakeAuthState(ctx)

		Convey("fail if IncludedTryjobs and OverriddenTryjobs are both provided", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &incompatibleTryjobOptions{
					hasIncludedTryjobs:   true,
					hasOverriddenTryjobs: true,
				},
			})
		})

		Convey("with a minimal test case", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			Convey("with a single CL", func() {})
			Convey("with multiple CLs", func() { in.addCL(userB.Email()) })
			res, err := Compute(ctx, *in)

			So(err, ShouldBeNil)
			So(res.ComputationFailure, ShouldBeNil)
			So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
				RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
					SingleQuota: 2,
					GlobalQuota: 8,
				},
				Definitions: []*tryjob.Definition{{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "test-proj",
								Bucket:  "test",
								Builder: "builder1",
							},
						},
					},
					Critical: true,
				}},
			})
		})
		Convey("includes undefined builder", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:unlisted")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &buildersNotDefined{
					Builders: []string{"test-proj/test/unlisted"},
				},
			})
		})
		Convey("includes unauthorized builder", func() {
			Convey("with single unauthorized user", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "test-proj/test/builder1",
						Allowlist: group2,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "test-proj/test/builder1",
					},
				})
			})
			Convey("with multiple users, one of which is unauthorized", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "test-proj/test/builder1",
						Allowlist: group1,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")
				// Add a second CL, the owner of which is not authorized to trigger builder1
				in.addCL(userD.Email())

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userD.Email()},
						Builder: "test-proj/test/builder1",
					},
				})
			})
		})
		Convey("with includable-only builder", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/builder1"}.generate(),
				builderConfigGenerator{Name: "test-proj/test.bucket/builder2", IncludableOnly: true}.generate(),
			})

			Convey("skips by default", func() {
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Definitions: []*tryjob.Definition{{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: "cr-buildbucket.appspot.com",
								Builder: &buildbucketpb.BuilderID{
									Project: "test-proj",
									Bucket:  "test",
									Builder: "builder1",
								},
							},
						},
						Critical: true,
					}},
				})
			})

			Convey("included", func() {
				Convey("modern style", func() {
					in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test.bucket:builder2")
				})
				Convey("legacy style", func() {
					in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci.test-proj.test.bucket:builder2")
				})
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Definitions: []*tryjob.Definition{
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test",
										Builder: "builder1",
									},
								},
							},
							Critical: true,
						},
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test.bucket",
										Builder: "builder2",
									},
								},
							},
							Critical: true,
						},
					},
				})
			})
		})
		Convey("includes equivalent builder explicitly", func() {
			Convey("unauthorized", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:          "test-proj/test/builder1",
					Allowlist:     "secret-group",
					EquiName:      "test-proj/test/equibuilder",
					EquiAllowlist: "other-secret-group",
				}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:equibuilder")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "test-proj/test/equibuilder",
					},
				})
			})

			Convey("authorized", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:          "test-proj/test/builder1",
					Allowlist:     "secret-group",
					EquiName:      "test-proj/test/equibuilder",
					EquiAllowlist: "", // Allow everyone
				}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:equibuilder")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Definitions: []*tryjob.Definition{{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: "cr-buildbucket.appspot.com",
								Builder: &buildbucketpb.BuilderID{
									Project: "test-proj",
									Bucket:  "test",
									Builder: "equibuilder",
								},
							},
						},
						Critical: true,
					}},
				})
			})
		})
		Convey("owner allowlist denied", func() {
			Convey("without equivalent builder", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:      "test-proj/test/builder1",
					Allowlist: "secret-group",
				}.generate()})

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeTrue)
				So(res.Requirement.GetDefinitions(), ShouldBeEmpty)
			})

			Convey("with equivalent builder", func() {
				Convey("equivalent builder allowed", func() {
					in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "", // allow everyone
					}.generate()})
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.OK(), ShouldBeTrue)
					So(res.Requirement.Definitions, ShouldResembleProto, []*tryjob.Definition{
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test",
										Builder: "equibuilder",
									},
								},
							},
							Critical: true,
						},
					})
				})

				Convey("equivalent builder denied", func() {
					in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "another-secret-group",
					}.generate()})
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.OK(), ShouldBeTrue)
					So(res.Requirement.GetDefinitions(), ShouldBeEmpty)
				})

			})
		})
		Convey("optional", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/optional-builder", ExperimentPercentage: 20}.generate(),
			})

			selected := 0

			baseCLID := int(in.CLs[0].ID)
			for i := 0; i < 1000; i++ {
				in.CLs[0].ID = common.CLID(baseCLID + i)
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				if len(res.Requirement.GetDefinitions()) > 0 {
					So(res.Requirement.GetDefinitions(), ShouldHaveLength, 1)
					def := res.Requirement.GetDefinitions()[0]
					So(def.GetCritical(), ShouldBeFalse)
					So(def.GetOptional(), ShouldBeTrue)
					selected++
				}
			}
			So(selected, ShouldBeBetween, 150, 250) // expecting 1000*20%=200
		})

		Convey("optional but explicitly included ", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/optional-builder", ExperimentPercentage: 0.001}.generate(),
			})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:optional-builder")

			baseCLID := int(in.CLs[0].ID)
			for i := 0; i < 10; i++ { // should include the definition all the time.
				in.CLs[0].ID = common.CLID(baseCLID + i)
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.Requirement.GetDefinitions(), ShouldHaveLength, 1)
				def := res.Requirement.GetDefinitions()[0]
				So(def.GetCritical(), ShouldBeTrue)
				So(def.GetOptional(), ShouldBeTrue)
			}
		})

		Convey("with location matching", func() {
			Convey("empty change after location exclusions skips builder", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name: "test-proj/test/builder1",
					LocationFilters: []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
						{
							GerritHostRegexp:    "example.com",
							GerritProjectRegexp: "repo",
							PathRegexp:          "some/.+",
							Exclude:             true,
						},
					},
				}.generate()})

				in.CLs[0].Detail.GetGerrit().Files = []string{
					"some/directory/contains/some/file",
				}
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
				})
			})
			Convey("with location filters", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name: "test-proj/test/builder1",
					LocationFilters: []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
						{
							GerritHostRegexp:    "example.com",
							GerritProjectRegexp: "repo",
							PathRegexp:          "some/.+",
						},
					},
				}.generate()})

				Convey("matching CL", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/directory/contains/some/file",
					}
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					// If the builder is not skipped, it will be in
					// res.Requirement.Definitions.
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
						Definitions: []*tryjob.Definition{{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test",
										Builder: "builder1",
									},
								},
							}, Critical: true,
						}},
					})
				})
				Convey("non-matching CL", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"other/directory/contains/some/file",
					}
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					})
				})
				Convey("CL with merge commit", func() {
					// No files changed, and two parents of the commit of the current revision.
					// This simulates a merge commit. See crbug/1006534.
					in.CLs[0].Detail.GetGerrit().Files = []string{}
					in.CLs[0].Detail.GetGerrit().Info = gf.CI(10, gf.ParentCommits([]string{"one", "two"}), gf.Project("repo"))
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
						Definitions: []*tryjob.Definition{{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test",
										Builder: "builder1",
									},
								},
							},
							Critical: true,
						}},
					})
				})
			})
			Convey("multi-CL, one CL with empty filediff, with location_filters", func() {
				// This test case is the same as the above, but using
				// location_filters, to test that the behavior is the same for
				// both.
				multiCLIn := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name: "luci/test/builder1",
					LocationFilters: []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
						{
							GerritHostRegexp:    "example.com",
							GerritProjectRegexp: "repo",
							PathRegexp:          "some/.+",
						},
					},
				}.generate()})
				multiCLIn.CLs[0].Detail.GetGerrit().Files = []string{
					"other/directory/contains/some/file",
				}
				multiCLIn.addCL(userA.Email())
				multiCLIn.CLs[1].Detail.GetGerrit().Files = []string{}
				multiCLIn.CLs[1].Detail.GetGerrit().Host = "example.com"
				multiCLIn.CLs[1].Detail.GetGerrit().Info = gf.CI(10, gf.ParentCommits([]string{"one", "two"}), gf.Project("repo"))
				res, err := Compute(ctx, *multiCLIn)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				// Builder is triggered because there is a merge commit.
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Definitions: []*tryjob.Definition{{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: "cr-buildbucket.appspot.com",
								Builder: &buildbucketpb.BuilderID{
									Project: "luci",
									Bucket:  "test",
									Builder: "builder1",
								},
							},
						},
						Critical: true,
					}},
				})
			})
			Convey("with location filters and exclusion", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name: "test-proj/test/builder1",
						LocationFilters: []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
							{
								GerritHostRegexp:    "example.com",
								GerritProjectRegexp: "repo",
								PathRegexp:          "some/.+",
								Exclude:             false,
							},
							{
								GerritHostRegexp:    "example.com",
								GerritProjectRegexp: "repo",
								PathRegexp:          "some/excluded/.*",
								Exclude:             true,
							},
						},
					}.generate()},
				)
				Convey("matching CL skipping builder", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
					}
					res, err := Compute(ctx, *in)

					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					})
				})
				Convey("partially matching CL skipping builder", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
						"unknown/path",
					}
					res, err := Compute(ctx, *in)

					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					})
				})
				Convey("matching CL not skipping builder", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
						"some/readme.md",
					}
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
						Definitions: []*tryjob.Definition{{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "test-proj",
										Bucket:  "test",
										Builder: "builder1",
									},
								},
							},
							Critical: true,
						}},
					})
				})
			})
		})

		Convey("stale check", func() {
			Convey("from config", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name: "test-proj/test/stale-default",
					}.generate(),
					builderConfigGenerator{
						Name:        "test-proj/test/stale-no",
						CancelStale: cfgpb.Toggle_NO,
					}.generate(),
					builderConfigGenerator{
						Name:        "test-proj/test/stale-yes",
						CancelStale: cfgpb.Toggle_YES,
					}.generate(),
				})

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement.GetDefinitions(), ShouldHaveLength, 3)
				expectedSkipStaleCheck := []bool{false, true, false}
				for i, def := range res.Requirement.GetDefinitions() {
					So(def.GetSkipStaleCheck(), ShouldEqual, expectedSkipStaleCheck[i])
				}
			})
			Convey("overridden by run option", func() {
				in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:        "test-proj/test/stale-no",
						CancelStale: cfgpb.Toggle_NO,
					}.generate(),
					builderConfigGenerator{
						Name:        "test-proj/test/stale-yes",
						CancelStale: cfgpb.Toggle_YES,
					}.generate(),
				})
				in.RunOptions = &run.Options{
					AvoidCancellingTryjobs: true,
				}

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				for _, def := range res.Requirement.GetDefinitions() {
					So(def.GetSkipStaleCheck(), ShouldBeTrue)
				}
			})
		})
		Convey("ignores included builders if in NPR mode", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{
					Name:  "test-proj/test/builder1",
					Modes: []string{string(run.NewPatchsetRun)},
				}.generate(),

				builderConfigGenerator{
					Name:           "test-proj/test.bucket/builder2",
					IncludableOnly: true,
				}.generate(),
			})
			in.RunMode = run.NewPatchsetRun
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test.bucket:builder2")
			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.ComputationFailure, ShouldBeNil)
			So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
				RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
					SingleQuota: 2,
					GlobalQuota: 8,
				},
				Definitions: []*tryjob.Definition{{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "test-proj",
								Bucket:  "test",
								Builder: "builder1",
							},
						},
					},
					Critical: true,
				}},
			})
		})

		Convey("Experiments", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.ConfigGroup.TryjobExperiments = []*cfgpb.ConfigGroup_TryjobExperiment{
				{Name: "experiment.a"}, // unconditional
				{
					Name: "experiment.b",
					Condition: &cfgpb.ConfigGroup_TryjobExperiment_Condition{
						OwnerGroupAllowlist: nil, // empty list
					},
				},
				{
					Name: "experiment.c",
					Condition: &cfgpb.ConfigGroup_TryjobExperiment_Condition{
						OwnerGroupAllowlist: []string{group1}, // CL owner is in group1
					},
				},
				{
					Name: "experiment.d",
					Condition: &cfgpb.ConfigGroup_TryjobExperiment_Condition{
						OwnerGroupAllowlist: []string{group2}, // CL owner is not in group2
					},
				},
			}
			res, err := Compute(ctx, *in)

			So(err, ShouldBeNil)
			So(res.ComputationFailure, ShouldBeNil)
			So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
				RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
					SingleQuota: 2,
					GlobalQuota: 8,
				},
				Definitions: []*tryjob.Definition{{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "test-proj",
								Bucket:  "test",
								Builder: "builder1",
							},
						},
					},
					Critical:    true,
					Experiments: []string{"experiment.a", "experiment.b", "experiment.c"},
				}},
			})
		})

		Convey("override has undefined builder", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:unlisted")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &buildersNotDefined{
					Builders: []string{"test-proj/test/unlisted"},
				},
			})
		})

		Convey("override honors SkipTryjobs option", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")
			in.RunOptions.SkipTryjobs = true

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeTrue)
			So(res.Requirement.GetDefinitions(), ShouldBeEmpty)
		})

		Convey("override has unauthorized Tryjob", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
				Name:      "test-proj/test/builder1",
				Allowlist: "secret-group",
			}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &unauthorizedIncludedTryjob{
					Users:   []string{userA.Email()},
					Builder: "test-proj/test/builder1",
				},
			})
		})

		Convey("override works", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{
					Name: "test-proj/test/builder1",
				}.generate(),
				builderConfigGenerator{
					Name: "test-proj/test/builder2",
				}.generate(),
			})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeTrue)
			So(res.Requirement.Definitions, ShouldResembleProto, []*tryjob.Definition{
				{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "test-proj",
								Bucket:  "test",
								Builder: "builder1",
							},
						},
					},
					Critical: true,
				},
			})
		})

		Convey("override works for equivalent builder", func() {
			in := makeInput(ctx, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{
					Name:     "test-proj/test/builder1",
					EquiName: "test-proj/test/builder2",
				}.generate(),
				builderConfigGenerator{
					Name: "test-proj/test/builder3",
				}.generate(),
			})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder2")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeTrue)
			So(res.Requirement.Definitions, ShouldHaveLength, 1)
			So(res.Requirement.Definitions[0].GetBuildbucket().GetBuilder().GetBuilder(), ShouldEqual, "builder2")
		})
	})
}

type builderConfigGenerator struct {
	Name                 string
	Allowlist            string
	IncludableOnly       bool
	EquiName             string
	EquiAllowlist        string
	ExperimentPercentage float32
	LocationFilters      []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter
	CancelStale          cfgpb.Toggle
	Modes                []string
}

func (bcg builderConfigGenerator) generate() *cfgpb.Verifiers_Tryjob_Builder {
	ret := &cfgpb.Verifiers_Tryjob_Builder{
		Name:            bcg.Name,
		IncludableOnly:  bcg.IncludableOnly,
		LocationFilters: bcg.LocationFilters,
		CancelStale:     bcg.CancelStale,
	}
	if len(bcg.Modes) != 0 {
		ret.ModeAllowlist = bcg.Modes
	}
	if bcg.Allowlist != "" {
		ret.OwnerWhitelistGroup = []string{bcg.Allowlist}
	}
	if bcg.EquiName != "" {
		ret.EquivalentTo = &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
			Name:                bcg.EquiName,
			OwnerWhitelistGroup: bcg.EquiAllowlist,
		}
	}
	if bcg.ExperimentPercentage != 0 {
		ret.ExperimentPercentage = bcg.ExperimentPercentage
	}
	return ret
}

func makeInput(ctx context.Context, builders []*cfgpb.Verifiers_Tryjob_Builder) *Input {
	ret := &Input{
		ConfigGroup: &cfgpb.ConfigGroup{
			Verifiers: &cfgpb.Verifiers{
				Tryjob: &cfgpb.Verifiers_Tryjob{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Builders: builders,
				},
			},
		},
		RunOwner:   userA,
		RunMode:    run.DryRun,
		RunOptions: &run.Options{},
		CLs: []*run.RunCL{
			{
				ID:         common.CLID(65566771212885957),
				ExternalID: changelist.MustGobID("example.com", 123456789),
				Trigger: &run.Trigger{
					Time: &timestamppb.Timestamp{Seconds: 1645080386},
				},
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info: &gerritpb.ChangeInfo{
								Owner: &gerritpb.AccountInfo{
									Email: userA.Email(),
								},
								Project: "repo",
							},
							Host:  "example.com",
							Files: []string{"readme.md"},
						},
					},
				},
			},
		},
	}
	return ret
}

func (in *Input) addCL(user string) {
	last := in.CLs[len(in.CLs)-1]
	host, id, _ := last.ExternalID.ParseGobID()
	in.CLs = append(in.CLs, &run.RunCL{
		ID:         last.ID + common.CLID(1),
		ExternalID: changelist.MustGobID(host, id+int64(1)),
		Trigger: &run.Trigger{
			Time: &timestamppb.Timestamp{Seconds: last.Trigger.Time.Seconds + int64(1)},
		},
		Detail: &changelist.Snapshot{
			Kind: &changelist.Snapshot_Gerrit{
				Gerrit: &changelist.Gerrit{
					Info: &gerritpb.ChangeInfo{
						Owner: &gerritpb.AccountInfo{
							Email: user,
						},
					},
				},
			},
		},
	})
}
