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

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
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
			So(isModeAllowed(run.DryRun, []string{string(run.FullRun), string(run.QuickDryRun)}), ShouldBeFalse)
		})
	})
}

func TestMakeDefinition(t *testing.T) {
	Convey("makeDefinition works", t, func() {
		valid := "a/b/c"
		alternateValid := "a/b/x"
		invalidShort := "d/e"
		invalidLong := "f/g/h/i"

		empty := ""

		def := makeDefinition(valid, empty, false, nonCritical)
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

		def = makeDefinition(valid, empty, true, critical)
		So(def, ShouldResembleProto, &tryjob.Definition{
			DisableReuse: true,
			Critical:     true,
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

		def = makeDefinition(valid, alternateValid, false, nonCritical)
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
		So(func() { makeDefinition(empty, empty, false, nonCritical) }, ShouldPanicLike, "unexpectedly empty")
		So(func() { makeDefinition(empty, valid, false, nonCritical) }, ShouldPanicLike, "unexpectedly empty")
		So(func() { makeBuildbucketDefinition(invalidShort) }, ShouldPanicLike, "unexpected format")
		So(func() { makeBuildbucketDefinition(invalidLong) }, ShouldPanicLike, "unexpected format")
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
	Convey("GetDisallowedOwners", t, func() {
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
		ctx, cancel := ct.SetUp()
		defer cancel()
		ctx = makeFakeAuthState(ctx)

		Convey("with a minimal test case", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "luci/test/builder1"}.generate()})
			Convey("with a single CL", func() {})
			Convey("with multiple CLs", func() { in.addCL(userB.Email()) })
			res, err := Compute(ctx, *in)

			So(err, ShouldBeNil)
			So(res.ComputationFailure, ShouldBeNil)
			So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
				RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
		Convey("includes undefined builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "luci/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:unlisted")

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &buildersNotDefined{
					Builders: []string{"luci/test/unlisted"},
				},
			})
		})
		Convey("includes triggeredBy builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "luci/test/builder1"}.generate(),
				builderConfigGenerator{Name: "luci/test/indirect", TriggeredBy: "luci/test/builder1"}.generate(),
			})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:indirect")
			res, err := Compute(ctx, *in)

			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &buildersNotDirectlyIncludable{
					Builders: []string{"luci/test/indirect"},
				},
			})
		})
		Convey("includes unauthorized builder", func() {
			Convey("with single unauthorized user", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "luci/test/builder1",
						Allowlist: group2,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:builder1")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "luci/test/builder1",
					},
				})
			})
			Convey("with multiple users, one of which is unauthorized", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:      "luci/test/builder1",
						Allowlist: group1,
					}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:builder1")
				// Add a second CL, the owner of which is not authorized to trigger builder1
				in.addCL(userD.Email())

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userD.Email()},
						Builder: "luci/test/builder1",
					},
				})
			})
		})
		Convey("with includable-only builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "luci/test/builder1"}.generate(),
				builderConfigGenerator{Name: "luci/test/builder2", IncludableOnly: true}.generate(),
			})

			Convey("skips by default", func() {
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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

			Convey("included", func() {
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:builder2")
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
					Definitions: []*tryjob.Definition{
						{
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
						},
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host: "cr-buildbucket.appspot.com",
									Builder: &buildbucketpb.BuilderID{
										Project: "luci",
										Bucket:  "test",
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
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:          "luci/test/builder1",
					Allowlist:     "secret-group",
					EquiName:      "luci/test/equibuilder",
					EquiAllowlist: "other-secret-group",
				}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:equibuilder")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{userA.Email()},
						Builder: "luci/test/equibuilder",
					},
				})
			})

			Convey("authorized", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:          "luci/test/builder1",
					Allowlist:     "secret-group",
					EquiName:      "luci/test/equibuilder",
					EquiAllowlist: "", // Allow everyone
				}.generate()})
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:equibuilder")

				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
									Builder: "equibuilder",
								},
							},
						},
						Critical: true,
					}},
				})
			})
		})
		Convey("experimental", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "luci/test/expbuilder", ExperimentPercentage: 100}.generate(),
				// The CLID and timestamp hardcoded in the mockBuilderConfig
				// generate function make this deterministically not selected.
				builderConfigGenerator{Name: "luci/test/expbuilder-notselected", ExperimentPercentage: 1}.generate(),
			})

			res, err := Compute(ctx, *in)
			So(err, ShouldBeNil)
			So(res.ComputationFailure, ShouldBeNil)
			So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
				RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
								Builder: "expbuilder",
							},
						},
					}},
				},
			})
		})
		Convey("with location matching", func() {
			Convey("empty change after location exclusions skips builder", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:                  "luci/test/builder1",
					LocationRegexpExclude: []string{"https://example.com/repo/[+]/some/.+"},
				}.generate()})

				in.CLs[0].Detail.GetGerrit().Files = []string{
					"some/directory/contains/some/file",
				}
				res, err := Compute(ctx, *in)
				So(err, ShouldBeNil)
				So(res.ComputationFailure, ShouldBeNil)
				So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
					RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
						SingleQuota: 2,
						GlobalQuota: 8,
					},
				})
			})
			Convey("with location regex", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{
					Name:           "luci/test/builder1",
					LocationRegexp: []string{"https://example.com/repo/[+]/some/.+"},
				}.generate()})

				Convey("matching CL", func() {
					in.CLs[0].Detail.GetGerrit().Files = []string{
						"some/directory/contains/some/file",
					}
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
					})
				})
				Convey("CL with empty filediff", func() {
					// No files changed. See crbug/1006534
					in.CLs[0].Detail.GetGerrit().Files = []string{}
					res, err := Compute(ctx, *in)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(err, ShouldBeNil)
					So(res.ComputationFailure, ShouldBeNil)
					So(res.Requirement, ShouldResembleProto, &tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
			})
			Convey("with location regex and exclusion", func() {
				in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
					builderConfigGenerator{
						Name:                  "luci/test/builder1",
						LocationRegexp:        []string{"https://example.com/repo/[+]/some/.+"},
						LocationRegexpExclude: []string{"https://example.com/repo/[+]/some/excluded/.*"},
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
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
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
			})
		})
	})
}

type builderConfigGenerator struct {
	Name                  string
	Allowlist             string
	IncludableOnly        bool
	EquiName              string
	EquiAllowlist         string
	ExperimentPercentage  float32
	LocationRegexp        []string
	LocationRegexpExclude []string
	TriggeredBy           string
}

func (bcg builderConfigGenerator) generate() *config.Verifiers_Tryjob_Builder {
	ret := &config.Verifiers_Tryjob_Builder{
		Name:                  bcg.Name,
		IncludableOnly:        bcg.IncludableOnly,
		LocationRegexp:        bcg.LocationRegexp,
		LocationRegexpExclude: bcg.LocationRegexpExclude,
		TriggeredBy:           bcg.TriggeredBy,
	}
	if bcg.Allowlist != "" {
		ret.OwnerWhitelistGroup = []string{bcg.Allowlist}
	}
	if bcg.EquiName != "" {
		ret.EquivalentTo = &config.Verifiers_Tryjob_EquivalentBuilder{
			Name:                bcg.EquiName,
			OwnerWhitelistGroup: bcg.EquiAllowlist,
		}
	}
	if bcg.ExperimentPercentage != 0 {
		ret.ExperimentPercentage = bcg.ExperimentPercentage
	}
	return ret
}

func makeInput(ctx context.Context, builders []*config.Verifiers_Tryjob_Builder) *Input {
	ret := &Input{
		ConfigGroup: &prjcfg.ConfigGroup{
			Content: &config.ConfigGroup{
				Verifiers: &config.Verifiers{
					Tryjob: &config.Verifiers_Tryjob{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
							GlobalQuota: 8,
						},
						Builders: builders,
					},
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
							Info: &gerrit.ChangeInfo{
								Owner: &gerrit.AccountInfo{
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
					Info: &gerrit.ChangeInfo{
						Owner: &gerrit.AccountInfo{
							Email: user,
						},
					},
				},
			},
		},
	})
}
