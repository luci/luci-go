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
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestIsModeAllowed(t *testing.T) {
	t.Run("isModeAllowed works", func(t *testing.T) {
		t.Run("when the mode is allowed", func(t *testing.T) {
			assert.That(t, isModeAllowed(run.DryRun, []string{string(run.FullRun), string(run.DryRun)}), should.BeTrue)
		})
		t.Run("when the mode is not allowed", func(t *testing.T) {
			assert.That(t, isModeAllowed(run.DryRun, []string{string(run.FullRun)}), should.BeFalse)
		})
	})
}

func TestDefinitionMaker(t *testing.T) {
	ftt.Run("definition maker works", t, func(t *ftt.Test) {
		valid := "a/b/c"
		alternateValid := "a/b/x"
		invalidShort := "d/e"
		invalidLong := "f/g/h/i"

		b := &cfgpb.Verifiers_Tryjob_Builder{
			Host: "buildbucket.example.com",
			Name: valid,
			EquivalentTo: &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
				Name: alternateValid,
			},
			ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_UNSET,
		}

		t.Run("main only", func(t *ftt.Test) {
			t.Run("flags off", func(t *ftt.Test) {
				def := (&definitionMaker{
					builder:     b,
					equivalence: mainOnly,
					criticality: nonCritical,
				}).make()
				assert.That(t, def, should.Match(&tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "buildbucket.example.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				}))
			})
			t.Run("flags on", func(t *ftt.Test) {
				b.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				b.ExperimentPercentage = 49.9
				b.DisableReuse = true
				b.DisableReuseFooters = []string{"Footer1", "Footer2"}
				def := (&definitionMaker{
					builder:     b,
					equivalence: mainOnly,
					criticality: critical,
				}).make()
				assert.That(t, def, should.Match(&tryjob.Definition{
					DisableReuse:        true,
					DisableReuseFooters: []string{"Footer1", "Footer2"},
					Critical:            true,
					Optional:            true,
					ResultVisibility:    cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED,
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "buildbucket.example.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				}))
			})
		})
		t.Run("equivalent only", func(t *ftt.Test) {
			def := (&definitionMaker{
				builder:     b,
				equivalence: equivalentOnly,
				criticality: nonCritical,
			}).make()
			assert.That(t, def, should.Match(&tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
						Builder: &buildbucketpb.BuilderID{
							Project: "a",
							Bucket:  "b",
							Builder: "x",
						},
					},
				},
			}))
		})
		t.Run("both", func(t *ftt.Test) {
			def := (&definitionMaker{
				builder:     b,
				equivalence: bothMainAndEquivalent,
				criticality: nonCritical,
			}).make()
			assert.That(t, def, should.Match(&tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
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
							Host: "buildbucket.example.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "x",
							},
						},
					},
				},
			}))
		})
		t.Run("flipped", func(t *ftt.Test) {
			def := (&definitionMaker{
				builder:     b,
				equivalence: flipMainAndEquivalent,
				criticality: nonCritical,
			}).make()
			assert.That(t, def, should.Match(&tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
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
							Host: "buildbucket.example.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "a",
								Bucket:  "b",
								Builder: "c",
							},
						},
					},
				},
			}))
		})

		t.Run("empty host name", func(t *ftt.Test) {
			b.Host = ""
			def := (&definitionMaker{
				builder:     b,
				equivalence: mainOnly,
				criticality: nonCritical,
			}).make()
			assert.That(t, def, should.Match(&tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: chromeinfra.BuildbucketHost,
						Builder: &buildbucketpb.BuilderID{
							Project: "a",
							Bucket:  "b",
							Builder: "c",
						},
					},
				},
			}))
		})
		t.Run("empty buildername in main", func(t *ftt.Test) {
			b.Name = ""
			dm := &definitionMaker{
				builder:     b,
				equivalence: mainOnly,
				criticality: critical,
			}
			assert.That(t, func() { dm.make() }, should.PanicLikeString("unexpectedly empty"))
		})
		t.Run("empty buildername in equivalent", func(t *ftt.Test) {
			b.EquivalentTo.Name = ""
			dm := &definitionMaker{
				builder:     b,
				equivalence: equivalentOnly,
				criticality: critical,
			}
			assert.That(t, func() { dm.make() }, should.PanicLikeString("unexpectedly empty"))
		})
		t.Run("short buildername", func(t *ftt.Test) {
			assert.That(t, func() { makeBuildbucketDefinition("bb.example.com", invalidShort) }, should.PanicLikeString("unexpected format"))
		})
		t.Run("long buildername", func(t *ftt.Test) {
			assert.That(t, func() { makeBuildbucketDefinition("bb.example.com", invalidLong) }, should.PanicLikeString("unexpected format"))
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

func TestGetDisallowedOwners(t *testing.T) {
	ftt.Run("getDisallowedOwners", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			FakeDB: authtest.NewFakeDB(
				authtest.MockMembership(userA, group1),
				authtest.MockMembership(userB, group1),
				authtest.MockMembership(userD, group2),
			),
		})

		input := Input{GFactory: ct.GFactory(), CLs: []*run.RunCL{
			{
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: "foo-review.example.com",
					}},
					LuciProject: "foo",
				},
			},
		},
		}

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: userA.Email()},
		})

		t.Run("works", func(t *ftt.Test) {
			t.Run("with no allowlists", func(t *ftt.Test) {
				disallowed, err := getDisallowedOwners(ctx, input, []string{userA.Email()})
				assert.NoErr(t, err)
				assert.Loosely(t, disallowed, should.BeEmpty)
			})
		})
		t.Run("panics", func(t *ftt.Test) {
			t.Run("with nil users", func(t *ftt.Test) {
				assert.That(t, func() { _, _ = getDisallowedOwners(ctx, input, nil, group1) }, should.PanicLikeString("nil user"))
			})
			t.Run("with zero users", func(t *ftt.Test) {
				assert.That(t, func() { _, _ = getDisallowedOwners(ctx, input, []string{}, group1) }, should.PanicLikeString("nil user"))
			})
		})
	})
}

func TestCompute(t *testing.T) {
	ftt.Run("Compute works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		ct.AddMember(userA.Email(), group1)
		ct.AddMember(userB.Email(), group1)
		ct.AddMember(userD.Email(), group2)

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: userA.Email()},
		})

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: userB.Email()},
		})

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: userD.Email()},
		})

		t.Run("fail if IncludedTryjobs and OverriddenTryjobs are both provided", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeFalse)
			assert.Loosely(t, res.ComputationFailure, should.HaveType[*incompatibleTryjobOptions])
			failure := res.ComputationFailure.(*incompatibleTryjobOptions)
			assert.That(t, failure.hasIncludedTryjobs, should.BeTrue)
			assert.That(t, failure.hasOverriddenTryjobs, should.BeTrue)
		})

		t.Run("with a minimal test case", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			t.Run("with a single CL", func(t *ftt.Test) {})
			t.Run("with multiple CLs", func(t *ftt.Test) { in.addCL(userB.Email()) })
			res, err := Compute(ctx, *in)

			assert.NoErr(t, err)
			assert.Loosely(t, res.ComputationFailure, should.BeNil)
			assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
			}))
		})
		t.Run("includes undefined builder", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:unlisted")

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeFalse)
			assert.That(t, res, should.Match(&ComputationResult{
				ComputationFailure: &buildersNotDefined{
					Builders: []string{"test-proj/test/unlisted"},
				},
			}))
		})
		t.Run("includes unauthorized builder", func(t *ftt.Test) {
			t.Run("with single unauthorized user", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "test-proj/test/builder1",
						Allowlist: group2,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")

				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.That(t, res.OK(), should.BeFalse)
				assert.That(t, res, should.Match(&ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "test-proj/test/builder1",
					},
				}))
			})
			t.Run("with multiple users, one of which is unauthorized", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "test-proj/test/builder1",
						Allowlist: group1,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")
				// Add a second CL, the owner of which is not authorized to trigger builder1
				in.addCL(userD.Email())

				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.That(t, res.OK(), should.BeFalse)
				assert.That(t, res, should.Match(&ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userD.Email()},
						Builder: "test-proj/test/builder1",
					},
				}))
			})
			t.Run("but can trigger if owners have bb schedule permission", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "test-proj/test/builder1",
						Allowlist: group1,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:builder1")
				// Add a second CL, the owner of which is not authorized to trigger builder1
				in.addCL(userD.Email())

				for _, owner := range in.allCLOwnersSorted() {
					ct.AddPermission(owner, bbperms.BuildsAdd, realms.Join("test-proj", "test"))
				}

				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.That(t, res.OK(), should.BeTrue)
			})
		})
		t.Run("with includable-only builder", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/builder1"}.generate(),
				builderConfigGenerator{Name: "test-proj/test.bucket/builder2", IncludableOnly: true}.generate(),
			})

			t.Run("skips by default", func(t *ftt.Test) {
				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.Loosely(t, res.ComputationFailure, should.BeNil)
				assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
				}))
			})

			t.Run("included", func(t *ftt.Test) {
				check := func(t testing.TB) {
					t.Helper()

					res, err := Compute(ctx, *in)
					assert.Loosely(t, err, should.BeNil, truth.LineContext())
					assert.Loosely(t, res.ComputationFailure, should.BeNil, truth.LineContext())
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
					}), truth.LineContext())
				}

				t.Run("modern style", func(t *ftt.Test) {
					in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test.bucket:builder2")
					check(t)
				})

				t.Run("legacy style", func(t *ftt.Test) {
					in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci.test-proj.test.bucket:builder2")
					check(t)
				})

			})
		})

		t.Run("includes equivalent builder explicitly", func(t *ftt.Test) {
			t.Run("unauthorized", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:          "test-proj/test/builder1",
					Allowlist:     "secret-group",
					EquiName:      "test-proj/test/equibuilder",
					EquiAllowlist: "other-secret-group",
				}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:equibuilder")

				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.That(t, res.OK(), should.BeFalse)
				assert.That(t, res, should.Match(&ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "test-proj/test/equibuilder",
					},
				}))
			})

			t.Run("authorized", func(t *ftt.Test) {
				var in *Input

				check := func(t testing.TB) {
					t.Helper()

					in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:equibuilder")

					res, err := Compute(ctx, *in)
					assert.Loosely(t, err, should.BeNil, truth.LineContext())
					assert.Loosely(t, res.ComputationFailure, should.BeNil, truth.LineContext())
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
					}), truth.LineContext())
				}

				t.Run("through allowlist group", func(t *ftt.Test) {
					in = makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "", // Allow everyone
					}.generate()})

					check(t)
				})

				t.Run("through buildbucket schedule permission", func(t *ftt.Test) {
					in = makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "other-secret-group",
					}.generate()})
					for _, owner := range in.allCLOwnersSorted() {
						ct.AddPermission(owner, bbperms.BuildsAdd, realms.Join("test-proj", "test"))
					}
					check(t)
				})

			})
		})

		t.Run("owner allowlist denied", func(t *ftt.Test) {
			t.Run("without equivalent builder", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:      "test-proj/test/builder1",
					Allowlist: "secret-group",
				}.generate()})

				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.That(t, res.OK(), should.BeTrue)
				assert.Loosely(t, res.Requirement.GetDefinitions(), should.BeEmpty)
			})

			t.Run("with equivalent builder", func(t *ftt.Test) {
				t.Run("equivalent builder allowed", func(t *ftt.Test) {
					in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "", // allow everyone
					}.generate()})
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.That(t, res.OK(), should.BeTrue)
					assert.That(t, res.Requirement.Definitions, should.Match([]*tryjob.Definition{
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
					}))
				})

				t.Run("equivalent builder denied", func(t *ftt.Test) {
					in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
						Name:          "test-proj/test/builder1",
						Allowlist:     "secret-group",
						EquiName:      "test-proj/test/equibuilder",
						EquiAllowlist: "another-secret-group",
					}.generate()})
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.That(t, res.OK(), should.BeTrue)
					assert.Loosely(t, res.Requirement.GetDefinitions(), should.BeEmpty)
				})

			})
		})
		t.Run("optional", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/optional-builder", ExperimentPercentage: 20}.generate(),
			})

			selected := 0

			baseCLID := int(in.CLs[0].ID)
			for i := range 1000 {
				in.CLs[0].ID = common.CLID(baseCLID + i)
				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				if len(res.Requirement.GetDefinitions()) > 0 {
					assert.Loosely(t, res.Requirement.GetDefinitions(), should.HaveLength(1))
					def := res.Requirement.GetDefinitions()[0]
					assert.That(t, def.GetCritical(), should.BeFalse)
					assert.That(t, def.GetOptional(), should.BeTrue)
					selected++
				}
			}
			assert.That(t, selected, should.BeBetween(150, 250)) // expecting 1000*20%=200
		})

		t.Run("optional but explicitly included ", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "test-proj/test/optional-builder", ExperimentPercentage: 0.001}.generate(),
			})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "test-proj/test:optional-builder")

			baseCLID := int(in.CLs[0].ID)
			for i := range 10 { // should include the definition all the time.
				in.CLs[0].ID = common.CLID(baseCLID + i)
				res, err := Compute(ctx, *in)
				assert.NoErr(t, err)
				assert.Loosely(t, res.Requirement.GetDefinitions(), should.HaveLength(1))
				def := res.Requirement.GetDefinitions()[0]
				assert.That(t, def.GetCritical(), should.BeTrue)
				assert.That(t, def.GetOptional(), should.BeTrue)
			}
		})

		t.Run("with location matching", func(t *ftt.Test) {
			t.Run("empty change after location exclusions skips builder", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
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
				assert.NoErr(t, err)
				assert.Loosely(t, res.ComputationFailure, should.BeNil)
				assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
					RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
				}))
			})
			t.Run("with location filters", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name: "test-proj/test/builder1",
					LocationFilters: []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
						{
							GerritHostRegexp:    "example.com",
							GerritProjectRegexp: "repo",
							PathRegexp:          "some/.+",
						},
					},
				}.generate()})

				t.Run("matching CL", func(t *ftt.Test) {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/directory/contains/some/file",
					}
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					// If the builder is not skipped, it will be in
					// res.Requirement.Definitions.
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
					}))
				})
				t.Run("non-matching CL", func(t *ftt.Test) {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"other/directory/contains/some/file",
					}
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					}))
				})
				t.Run("CL with merge commit", func(t *ftt.Test) {
					// No files changed, and two parents of the commit of the current revision.
					// This simulates a merge commit. See crbug/1006534.
					in.CLs[0].Detail.GetGerrit().Files = []string{}
					in.CLs[0].Detail.GetGerrit().Info = gf.CI(10, gf.ParentCommits([]string{"one", "two"}), gf.Project("repo"))
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
					}))
				})
			})
			t.Run("multi-CL, one CL with empty filediff, with location_filters", func(t *ftt.Test) {
				// This test case is the same as the above, but using
				// location_filters, to test that the behavior is the same for
				// both.
				multiCLIn := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
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
				assert.NoErr(t, err)
				assert.Loosely(t, res.ComputationFailure, should.BeNil)
				// Builder is triggered because there is a merge commit.
				assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
				}))
			})
			t.Run("with location filters and exclusion", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
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
				t.Run("matching CL skipping builder", func(t *ftt.Test) {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
					}
					res, err := Compute(ctx, *in)

					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					}))
				})
				t.Run("partially matching CL skipping builder", func(t *ftt.Test) {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
						"unknown/path",
					}
					res, err := Compute(ctx, *in)

					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					}))
				})
				t.Run("matching CL not skipping builder", func(t *ftt.Test) {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/excluded/file",
						"some/readme.md",
					}
					res, err := Compute(ctx, *in)
					assert.NoErr(t, err)
					assert.Loosely(t, res.ComputationFailure, should.BeNil)
					assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
					}))
				})
			})
		})

		t.Run("stale check", func(t *ftt.Test) {
			t.Run("from config", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
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
				assert.NoErr(t, err)
				assert.Loosely(t, res.ComputationFailure, should.BeNil)
				assert.Loosely(t, res.Requirement.GetDefinitions(), should.HaveLength(3))
				expectedSkipStaleCheck := []bool{false, true, false}
				for i, def := range res.Requirement.GetDefinitions() {
					assert.That(t, def.GetSkipStaleCheck(), should.Equal(expectedSkipStaleCheck[i]))
				}
			})
			t.Run("overridden by run option", func(t *ftt.Test) {
				in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
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
				assert.NoErr(t, err)
				assert.Loosely(t, res.ComputationFailure, should.BeNil)
				for _, def := range res.Requirement.GetDefinitions() {
					assert.That(t, def.GetSkipStaleCheck(), should.BeTrue)
				}
			})
		})
		t.Run("ignores included builders if in NPR mode", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
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
			assert.NoErr(t, err)
			assert.Loosely(t, res.ComputationFailure, should.BeNil)
			assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
			}))
		})

		t.Run("Experiments", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
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

			assert.NoErr(t, err)
			assert.Loosely(t, res.ComputationFailure, should.BeNil)
			assert.That(t, res.Requirement, should.Match(&tryjob.Requirement{
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
			}))
		})

		t.Run("Has skip footers", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				{
					Name: "test-proj/test/builder1",
					SkipFooters: []*cfgpb.Verifiers_Tryjob_Builder_SkipFooter{
						{
							Key:         "Bypass-Builder-1",
							ValueRegexp: `^(?i)(true)$`,
						},
					},
				},
				{
					Name: "test-proj/test/builder2",
					SkipFooters: []*cfgpb.Verifiers_Tryjob_Builder_SkipFooter{
						{
							Key:         "Bypass-Builder-2",
							ValueRegexp: `^(?i)(true)$`,
						},
					},
				},
			})
			in.CLs[0].Detail.Metadata = append(in.CLs[0].Detail.Metadata,
				&changelist.StringPair{
					Key:   "Bypass-Builder-1",
					Value: "True",
				},
				&changelist.StringPair{
					Key:   "Bypass-Builder-2",
					Value: "false",
				},
			)

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.Loosely(t, res.ComputationFailure, should.BeNil)
			assert.That(t, res.Requirement.Definitions, should.Match([]*tryjob.Definition{
				{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host: "cr-buildbucket.appspot.com",
							Builder: &buildbucketpb.BuilderID{
								Project: "test-proj",
								Bucket:  "test",
								Builder: "builder2",
							},
						},
					},
					Critical: true,
				},
			}))
		})
		t.Run("override has undefined builder", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:unlisted")

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeFalse)
			assert.That(t, res, should.Match(&ComputationResult{
				ComputationFailure: &buildersNotDefined{
					Builders: []string{"test-proj/test/unlisted"},
				},
			}))
		})

		t.Run("override honors SkipTryjobs option", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "test-proj/test/builder1"}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")
			in.RunOptions.SkipTryjobs = true

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeTrue)
			assert.Loosely(t, res.Requirement.GetDefinitions(), should.BeEmpty)
		})

		t.Run("override honors skip footers", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				{
					Name: "test-proj/test/builder1",
					SkipFooters: []*cfgpb.Verifiers_Tryjob_Builder_SkipFooter{
						{
							Key:         "Bypass-Builder-1",
							ValueRegexp: `^(?i)(true)$`,
						},
					},
				},
			})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")
			in.CLs[0].Detail.Metadata = append(in.CLs[0].Detail.Metadata,
				&changelist.StringPair{
					Key:   "Bypass-Builder-1",
					Value: "True",
				},
			)
			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeTrue)
			assert.Loosely(t, res.Requirement.GetDefinitions(), should.BeEmpty)
		})

		t.Run("override has unauthorized Tryjob", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{builderConfigGenerator{
				Name:      "test-proj/test/builder1",
				Allowlist: "secret-group",
			}.generate()})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeFalse)
			assert.That(t, res, should.Match(&ComputationResult{
				ComputationFailure: &unauthorizedIncludedTryjob{
					Users:   []string{userA.Email()},
					Builder: "test-proj/test/builder1",
				},
			}))
		})

		t.Run("override works", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
				builderConfigGenerator{
					Name: "test-proj/test/builder1",
				}.generate(),
				builderConfigGenerator{
					Name: "test-proj/test/builder2",
				}.generate(),
			})
			in.RunOptions.OverriddenTryjobs = append(in.RunOptions.OverriddenTryjobs, "test-proj/test:builder1")

			res, err := Compute(ctx, *in)
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeTrue)
			assert.That(t, res.Requirement.Definitions, should.Match([]*tryjob.Definition{
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
			}))
		})

		t.Run("override works for equivalent builder", func(t *ftt.Test) {
			in := makeInput(ctx, &ct, []*cfgpb.Verifiers_Tryjob_Builder{
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
			assert.NoErr(t, err)
			assert.That(t, res.OK(), should.BeTrue)
			assert.Loosely(t, res.Requirement.Definitions, should.HaveLength(1))
			assert.That(t, res.Requirement.Definitions[0].GetBuildbucket().GetBuilder().GetBuilder(), should.Equal("builder2"))
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

func makeInput(ctx context.Context, ct *cvtesting.Test, builders []*cfgpb.Verifiers_Tryjob_Builder) *Input {
	ret := &Input{
		GFactory: ct.GFactory(),
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
