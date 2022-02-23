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
	"go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/proto/gerrit"
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

		def := makeDefinition(valid, empty, false)
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

		def = makeDefinition(valid, empty, true)
		So(def, ShouldResembleProto, &tryjob.Definition{
			DisableReuse: true,
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

		def = makeDefinition(valid, alternateValid, false)
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
		So(func() { makeDefinition(empty, empty, false) }, ShouldPanicLike, "unexpectedly empty")
		So(func() { makeDefinition(empty, valid, false) }, ShouldPanicLike, "unexpectedly empty")
		So(func() { makeBuildbucketDefinition(invalidShort) }, ShouldPanicLike, "unexpected format")
		So(func() { makeBuildbucketDefinition(invalidLong) }, ShouldPanicLike, "unexpected format")
	})
}

func TestCompute(t *testing.T) {
	Convey("Compute works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("with a minimal test case", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "luci/test/builder1"}.generate()})
			res, err := Compute(ctx, in)

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
					}},
				},
			})
		})
		Convey("includes undefined builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{builderConfigGenerator{Name: "luci/test/builder1"}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:unlisted")

			res, err := Compute(ctx, in)
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
			res, err := Compute(ctx, in)

			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &buildersNotDirectlyIncludable{
					Builders: []string{"luci/test/indirect"},
				},
			})
		})
		Convey("includes unauthorized builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
				builderConfigGenerator{
					Name:      "luci/test/builder1",
					Allowlist: "secret-group",
				}.generate()})
			in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:builder1")

			res, err := Compute(ctx, in)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)
			So(res, ShouldResemble, &ComputationResult{
				ComputationFailure: &unauthorizedIncludedTryjob{
					Users:   []string{"someuser@example.com"},
					Builder: "luci/test/builder1",
				},
			})
		})
		Convey("with includable-only builder", func() {
			in := makeInput(ctx, []*config.Verifiers_Tryjob_Builder{
				builderConfigGenerator{Name: "luci/test/builder1"}.generate(),
				builderConfigGenerator{Name: "luci/test/builder2", IncludableOnly: true}.generate(),
			})

			Convey("skips by default", func() {
				res, err := Compute(ctx, in)
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
						}},
					},
				})
			})

			Convey("included", func() {
				in.RunOptions.IncludedTryjobs = append(in.RunOptions.IncludedTryjobs, "luci/test:builder2")
				res, err := Compute(ctx, in)
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

				res, err := Compute(ctx, in)
				So(err, ShouldBeNil)
				So(res.OK(), ShouldBeFalse)
				So(res, ShouldResemble, &ComputationResult{
					ComputationFailure: &unauthorizedIncludedTryjob{
						Users:   []string{"someuser@example.com"},
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

				res, err := Compute(ctx, in)
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
						}},
					},
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

			res, err := Compute(ctx, in)
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
	})
}

type builderConfigGenerator struct {
	Name                 string
	Allowlist            string
	IncludableOnly       bool
	EquiName             string
	EquiAllowlist        string
	ExperimentPercentage float32
	TriggeredBy          string
}

func (bcg builderConfigGenerator) generate() *config.Verifiers_Tryjob_Builder {
	ret := &config.Verifiers_Tryjob_Builder{
		Name:           bcg.Name,
		IncludableOnly: bcg.IncludableOnly,
		TriggeredBy:    bcg.TriggeredBy,
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

func makeInput(ctx context.Context, builders []*config.Verifiers_Tryjob_Builder) Input {
	return Input{
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
		RunOwner:   identity.Identity("user:someuser@example.com"),
		RunMode:    run.DryRun,
		RunOptions: &run.Options{},
		CLs: []*run.RunCL{
			{
				ID:         common.CLID(65566771212885957),
				ExternalID: changelist.MustGobID("test.googlesource.com", 123456789),
				Trigger: &run.Trigger{
					Time: &timestamppb.Timestamp{Seconds: 1645080386},
				},
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info: &gerrit.ChangeInfo{
								Owner: &gerrit.AccountInfo{
									Email: "someuser@example.com",
								},
							},
						},
					},
				},
			},
		},
	}
}
