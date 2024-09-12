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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLaunch(t *testing.T) {
	ftt.Run("Launch", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		f := &Facade{
			ClientFactory: ct.BuildbucketFake.NewClientFactory(),
		}

		const (
			gHost    = "example-review.googlesource.com"
			gRepo    = "repo/example"
			gChange1 = 11
			gChange2 = 22

			bbHost  = "buildbucket.example.com"
			bbHost2 = "buildbucket-2.example.com"

			lProject     = "testProj"
			owner1Email  = "owner1@example.com"
			owner2Email  = "owner2@example.com"
			triggerEmail = "triggerer@example.com"
		)
		ownerIdentity, err := identity.MakeIdentity(fmt.Sprintf("user:%s", owner1Email))
		assert.Loosely(t, err, should.BeNil)
		ct.AddMember(ownerIdentity.Email(), "googlers") // Run owner is a googler
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		ct.BuildbucketFake.AddBuilder(bbHost, builderID, map[string]any{
			"foo": "bar",
		})
		bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		assert.Loosely(t, err, should.BeNil)

		epoch := ct.Clock.Now().UTC()
		cl1 := &run.RunCL{
			ID:         1,
			ExternalID: changelist.MustGobID(gHost, gChange1),
			Detail: &changelist.Snapshot{
				Patchset:              4,
				MinEquivalentPatchset: 3,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: &gerritpb.ChangeInfo{
							Project: gRepo,
							Number:  gChange1,
							Owner: &gerritpb.AccountInfo{
								Email: owner1Email,
							},
						},
					},
				},
			},
			Trigger: &run.Trigger{
				Email: triggerEmail,
			},
		}
		cl2 := &run.RunCL{
			ID:         2,
			ExternalID: changelist.MustGobID(gHost, gChange2),
			Detail: &changelist.Snapshot{
				Patchset:              10,
				MinEquivalentPatchset: 10,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: &gerritpb.ChangeInfo{
							Project: gRepo,
							Number:  gChange2,
							Owner: &gerritpb.AccountInfo{
								Email: owner2Email,
							},
						},
					},
				},
			},
			Trigger: &run.Trigger{
				Email: triggerEmail,
			},
		}
		cls := []*run.RunCL{cl1, cl2}
		r := &run.Run{
			ID:    common.MakeRunID(lProject, epoch, 1, []byte("cafe")),
			Owner: ownerIdentity,
			CLs:   common.CLIDs{cl1.ID, cl2.ID},
			Mode:  run.DryRun,
			Options: &run.Options{
				CustomTryjobTags: []string{"foo:bar"},
			},
		}

		t.Run("Single Tryjob", func(t *ftt.Test) {
			definition := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host:    bbHost,
						Builder: builderID,
					},
				},
				Experiments: []string{"infra.experiment.foo", "infra.experiment.bar"},
			}
			t.Run("Not Optional", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					ID:         65535,
					Definition: definition,
					Status:     tryjob.Status_PENDING,
				}
				launchResults := f.Launch(ctx, []*tryjob.Tryjob{tj}, r, cls)
				assert.Loosely(t, launchResults, should.HaveLength(1))
				launchResult := launchResults[0]
				assert.Loosely(t, launchResult.Err, should.BeNil)
				assert.Loosely(t, launchResult.ExternalID, should.NotBeEmpty)
				host, id, err := launchResult.ExternalID.ParseBuildbucketID()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, host, should.Equal(bbHost))
				assert.Loosely(t, launchResult.Status, should.Equal(tryjob.Status_TRIGGERED))
				assert.Loosely(t, launchResult.Result, should.Resemble(&tryjob.Result{
					Status:     tryjob.Result_UNKNOWN,
					CreateTime: timestamppb.New(ct.Clock.Now()),
					UpdateTime: timestamppb.New(ct.Clock.Now()),
					Backend: &tryjob.Result_Buildbucket_{
						Buildbucket: &tryjob.Result_Buildbucket{
							Id:      id,
							Status:  bbpb.Status_SCHEDULED,
							Builder: builderID,
						},
					},
				}))
				build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: id,
					Mask: &bbpb.BuildMask{
						AllFields: true,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, build.GetBuilder(), should.Resemble(builderID))
				assert.Loosely(t, build.GetInput().GetProperties(), should.Resemble(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"foo": structpb.NewStringValue("bar"),
						propertyKey: structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"active":         structpb.NewBoolValue(true),
								"dryRun":         structpb.NewBoolValue(true),
								"topLevel":       structpb.NewBoolValue(true),
								"runMode":        structpb.NewStringValue(string(run.DryRun)),
								"ownerIsGoogler": structpb.NewBoolValue(true),
							},
						}),
						legacyPropertyKey: structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"active":         structpb.NewBoolValue(true),
								"dryRun":         structpb.NewBoolValue(true),
								"topLevel":       structpb.NewBoolValue(true),
								"runMode":        structpb.NewStringValue(string(run.DryRun)),
								"ownerIsGoogler": structpb.NewBoolValue(true),
							},
						}),
					},
				}))
				assert.Loosely(t, build.GetInput().GetGerritChanges(), should.Resemble([]*bbpb.GerritChange{
					{Host: gHost, Project: gRepo, Change: gChange1, Patchset: 4},
					{Host: gHost, Project: gRepo, Change: gChange2, Patchset: 10},
				}))
				assert.Loosely(t, build.GetTags(), should.Resemble([]*bbpb.StringPair{
					{Key: "cq_attempt_key", Value: "63616665"},
					{Key: "cq_cl_group_key", Value: "42497728aa4b5097"},
					{Key: "cq_cl_owner", Value: owner1Email},
					{Key: "cq_cl_owner", Value: owner2Email},
					{Key: "cq_cl_tag", Value: "foo:bar"},
					{Key: "cq_equivalent_cl_group_key", Value: "1aac15146c0bc164"},
					{Key: "cq_experimental", Value: "false"},
					{Key: "cq_triggerer", Value: triggerEmail},
					{Key: "user_agent", Value: "cq"},
				}))
				assert.Loosely(t, build.GetInput().GetExperiments(), should.Resemble([]string{"infra.experiment.bar", "infra.experiment.foo"}))
			})

			t.Run("Optional Tryjob", func(t *ftt.Test) {
				definition.Optional = true
				tj := &tryjob.Tryjob{
					ID:         65535,
					Definition: definition,
					Status:     tryjob.Status_PENDING,
				}
				launchResults := f.Launch(ctx, []*tryjob.Tryjob{tj}, r, cls)
				assert.Loosely(t, launchResults, should.HaveLength(1))
				launchResult := launchResults[0]
				assert.Loosely(t, launchResult.Err, should.BeNil)
				assert.Loosely(t, launchResult.ExternalID, should.NotBeEmpty)
				_, id, err := launchResult.ExternalID.ParseBuildbucketID()
				assert.Loosely(t, err, should.BeNil)
				build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: id,
					Mask: &bbpb.BuildMask{
						AllFields: true,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, build.GetInput().GetProperties().GetFields()[propertyKey].GetStructValue().GetFields()["experimental"].GetBoolValue(), should.BeTrue)
				var experimentalTag *bbpb.StringPair
				for _, tag := range build.GetTags() {
					if tag.GetKey() == "cq_experimental" {
						experimentalTag = tag
						break
					}
				}
				assert.Loosely(t, experimentalTag, should.NotBeNil)
				assert.Loosely(t, experimentalTag.GetValue(), should.Equal("true"))
			})
		})

		t.Run("Multiple across hosts", func(t *ftt.Test) {
			tryjobs := []*tryjob.Tryjob{
				{
					ID: 65533,
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "bucketFoo",
									Builder: "builderFoo",
								},
							},
						},
					},
					Status: tryjob.Status_PENDING,
				},
				{
					ID: 65534,
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "bucketBar",
									Builder: "builderBar",
								},
							},
						},
						Optional: true,
					},
					Status: tryjob.Status_PENDING,
				},
				{
					ID: 65535,
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost2,
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "bucketBaz",
									Builder: "builderBaz",
								},
							},
						},
					},
					Status: tryjob.Status_PENDING,
				},
			}
			ct.BuildbucketFake.AddBuilder(bbHost, tryjobs[0].Definition.GetBuildbucket().GetBuilder(), nil)
			ct.BuildbucketFake.AddBuilder(bbHost, tryjobs[1].Definition.GetBuildbucket().GetBuilder(), nil)
			ct.BuildbucketFake.AddBuilder(bbHost2, tryjobs[2].Definition.GetBuildbucket().GetBuilder(), nil)
			launchResults := f.Launch(ctx, tryjobs, r, cls)
			for i, launchResult := range launchResults {
				assert.Loosely(t, launchResult.Err, should.BeNil)
				assert.Loosely(t, launchResult.ExternalID, should.NotBeEmpty)
				host, id, err := launchResult.ExternalID.ParseBuildbucketID()
				assert.Loosely(t, err, should.BeNil)
				switch i {
				case 0, 1:
					assert.Loosely(t, host, should.Equal(bbHost))
				default:
					assert.Loosely(t, host, should.Equal(bbHost2))
				}
				bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, host, lProject)
				assert.Loosely(t, err, should.BeNil)
				build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: id,
					Mask: &bbpb.BuildMask{
						AllFields: true,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, build, should.NotBeNil)
			}
		})

		t.Run("Large number of tryjobs", func(t *ftt.Test) {
			tryjobs := make([]*tryjob.Tryjob, 2000)
			for i := range tryjobs {
				tryjobs[i] = &tryjob.Tryjob{
					ID: common.TryjobID(10000 + i),
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "bucketFoo",
									Builder: fmt.Sprintf("builderFoo-%d", i),
								},
							},
						},
					},
					Status: tryjob.Status_PENDING,
				}
				ct.BuildbucketFake.AddBuilder(bbHost, tryjobs[i].Definition.GetBuildbucket().GetBuilder(), nil)
			}
			launchResults := f.Launch(ctx, tryjobs, r, cls)
			assert.Loosely(t, launchResults, should.HaveLength(len(tryjobs)))
			for i, launchResult := range launchResults {
				assert.Loosely(t, launchResult.Err, should.BeNil)
				assert.Loosely(t, launchResult.ExternalID, should.NotBeEmpty)
				host, id, err := launchResult.ExternalID.ParseBuildbucketID()
				assert.Loosely(t, err, should.BeNil)
				bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, host, lProject)
				assert.Loosely(t, err, should.BeNil)
				build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: id,
					Mask: &bbpb.BuildMask{
						AllFields: true,
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, build.GetBuilder(), should.Resemble(tryjobs[i].Definition.GetBuildbucket().GetBuilder()))
			}
		})

		t.Run("Failure", func(t *ftt.Test) {
			tryjobs := []*tryjob.Tryjob{
				{
					ID: 65534,
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host:    bbHost,
								Builder: builderID,
							},
						},
					},
					Status: tryjob.Status_PENDING,
				},
				{
					ID: 65535,
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "testBucket",
									Builder: "anotherBuilder",
								},
							},
						},
					},
					Status: tryjob.Status_PENDING,
				},
			}
			launchResults := f.Launch(ctx, tryjobs, r, cls)
			assert.Loosely(t, launchResults, should.HaveLength(2))
			assert.Loosely(t, launchResults[0].Err, should.BeNil) // First Tryjob launched successfully
			assert.Loosely(t, launchResults[0].ExternalID, should.NotBeEmpty)
			assert.Loosely(t, launchResults[1].Err, convey.Adapt(ShouldBeRPCNotFound)())
			assert.Loosely(t, launchResults[1].ExternalID, should.BeEmpty)
			assert.Loosely(t, launchResults[1].Status, should.Equal(tryjob.Status_STATUS_UNSPECIFIED))
			assert.Loosely(t, launchResults[1].Result, should.BeNil)
		})

		t.Run("Deduplicate", func(t *ftt.Test) {
			tj := &tryjob.Tryjob{
				ID: 655365,
				Definition: &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host:    bbHost,
							Builder: builderID,
						},
					},
				},
				Status: tryjob.Status_PENDING,
			}
			launchResults := f.Launch(ctx, []*tryjob.Tryjob{tj}, r, cls)
			assert.Loosely(t, launchResults, should.HaveLength(1))
			assert.Loosely(t, launchResults[0].Err, should.BeNil)
			firstExternalID := launchResults[0].ExternalID
			ct.Clock.Add(10 * time.Second)
			launchResults = f.Launch(ctx, []*tryjob.Tryjob{tj}, r, cls)
			assert.Loosely(t, launchResults, should.HaveLength(1))
			assert.Loosely(t, launchResults[0].Err, should.BeNil)
			secondExternalID := launchResults[0].ExternalID
			assert.Loosely(t, secondExternalID, should.Equal(firstExternalID))
		})
	})
}
