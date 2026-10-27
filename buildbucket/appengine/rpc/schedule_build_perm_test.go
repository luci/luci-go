// Copyright 2026 The LUCI Authors.
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

package rpc

import (
	"context"
	"math/rand"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestHasCustomParams(t *testing.T) {
	t.Parallel()

	ftt.Run("hasCustomParams", t, func(t *ftt.Test) {
		builder := &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"}

		t.Run("no params", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				RequestId:        "request-id",
				TemplateBuildId:  1,
				Builder:          builder,
				Tags:             []*pb.StringPair{{Key: "user_agent", Value: "v"}},
				Notify:           &pb.NotificationConfig{PubsubTopic: "topic"},
				Mask:             &pb.BuildMask{AllFields: true},
				DryRun:           true,
				ParentBuildId:    1,
				CanOutliveParent: pb.Trinary_YES,
				Swarming:         &pb.ScheduleBuildRequest_Swarming{ParentRunId: "id"},
				// An empty map is not considered set.
				Experiments: map[string]bool{},
			}
			assert.Loosely(t, hasCustomParams(req), should.BeFalse)
		})

		cases := map[string]*pb.ScheduleBuildRequest{
			"properties": {Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{"a": structpb.NewStringValue("b")},
			}},
			"gitiles_commit":            {GitilesCommit: &pb.GitilesCommit{Host: "host"}},
			"gerrit_changes":            {GerritChanges: []*pb.GerritChange{{Host: "host"}}},
			"dimensions":                {Dimensions: []*pb.RequestedDimension{{Key: "k", Value: "v"}}},
			"experiments":               {Experiments: map[string]bool{"e": true}},
			"canary":                    {Canary: pb.Trinary_YES},
			"experimental":              {Experimental: pb.Trinary_YES},
			"priority":                  {Priority: 1},
			"critical":                  {Critical: pb.Trinary_YES},
			"exe":                       {Exe: &pb.Executable{CipdVersion: "v"}},
			"scheduling_timeout":        {SchedulingTimeout: durationpb.New(60)},
			"execution_timeout":         {ExecutionTimeout: durationpb.New(60)},
			"grace_period":              {GracePeriod: durationpb.New(60)},
			"retriable":                 {Retriable: pb.Trinary_NO},
			"shadow_input":              {ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{}},
			"resultdb":                  {Resultdb: &pb.ScheduleBuildRequest_ResultDB{IsExportRootOverride: true}},
			"tags":                      {Tags: []*pb.StringPair{{Key: "k", Value: "v"}}},
			"empty tag key":             {Tags: []*pb.StringPair{{Key: "", Value: "v"}}},
			"differently cased tag key": {Tags: []*pb.StringPair{{Key: "User_Agent", Value: "v"}}},
			"allowed and other tags": {Tags: []*pb.StringPair{
				{Key: "user_agent", Value: "v"},
				{Key: "k", Value: "v"},
			}},
		}
		for name, req := range cases {
			t.Run(name, func(t *ftt.Test) {
				req.Builder = builder
				assert.Loosely(t, hasCustomParams(req), should.BeTrue)
			})
		}
	})
}

func TestScheduleBuildPermissions(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:caller@example.com")

	ftt.Run("ScheduleBuild permissions", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, _ = tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.NoErr(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			SwarmingBackends: map[string]string{"host": "swarming://host"},
			Resultdb:         &pb.ResultDBSettings{Hostname: "rdbHost"},
			Swarming: &pb.SwarmingSettings{
				BbagentPackage: &pb.SwarmingSettings_Package{
					PackageName: "bbagent/${platform}",
					Version:     "bbagent-version",
				},
				KitchenPackage: &pb.SwarmingSettings_Package{
					PackageName: "kitchen",
					Version:     "kitchen-version",
				},
			},
		}))
		testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
		testutil.PutBuilder(ctx, "project", "bucket", "builder")

		// A previous build with custom properties, to be used as a template.
		tmpl := &model.Build{
			ID: 1000,
			Proto: &pb.Build{
				Id: 1000,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}
		assert.NoErr(t, datastore.Put(ctx, tmpl, &model.BuildInputProperties{
			Build: datastore.KeyForObj(ctx, tmpl),
			Proto: &structpb.Struct{
				Fields: map[string]*structpb.Value{"a": structpb.NewStringValue("b")},
			},
		}))

		withPerms := func(perms ...realms.Permission) context.Context {
			mocks := []authtest.MockedDatum{
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
				// Needed to use template_build_id.
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			}
			for _, p := range perms {
				mocks = append(mocks, authtest.MockPermission(userID, "project:bucket", p))
			}
			return auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB:   authtest.NewFakeDB(mocks...),
			})
		}

		plainReq := func() *pb.ScheduleBuildRequest {
			return &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				DryRun: true,
			}
		}
		paramsReq := func() *pb.ScheduleBuildRequest {
			req := plainReq()
			req.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{"a": structpb.NewStringValue("b")},
			}
			return req
		}
		templateReq := func() *pb.ScheduleBuildRequest {
			return &pb.ScheduleBuildRequest{
				TemplateBuildId: 1000,
				DryRun:          true,
			}
		}
		schedule := func(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
			blds, merr := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
			return blds[0], merr[0]
		}

		t.Run("no permissions", func(t *ftt.Test) {
			ctx := withPerms()

			_, err := schedule(ctx, plainReq())
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.add"`))

			_, err = schedule(ctx, paramsReq())
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.add"`))
		})

		t.Run("builds.add only", func(t *ftt.Test) {
			ctx := withPerms(bbperms.BuildsAdd)

			t.Run("without params", func(t *ftt.Test) {
				bld, err := schedule(ctx, plainReq())
				assert.NoErr(t, err)
				assert.Loosely(t, bld, should.NotBeNil)
			})

			t.Run("retry via template_build_id", func(t *ftt.Test) {
				bld, err := schedule(ctx, templateReq())
				assert.NoErr(t, err)
				assert.Loosely(t, bld.Input.Properties.Fields["a"].GetStringValue(), should.Equal("b"))
			})

			t.Run("with properties", func(t *ftt.Test) {
				_, err := schedule(ctx, paramsReq())
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.addWithParams"`))
			})

			t.Run("with allowed tags", func(t *ftt.Test) {
				req := plainReq()
				req.Tags = []*pb.StringPair{{Key: "user_agent", Value: "agent"}}
				_, err := schedule(ctx, req)
				assert.NoErr(t, err)
			})

			t.Run("with other tags", func(t *ftt.Test) {
				req := plainReq()
				req.Tags = []*pb.StringPair{
					{Key: "user_agent", Value: "agent"},
					{Key: "k", Value: "v"},
				}
				_, err := schedule(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.addWithParams"`))
			})

			t.Run("with other params", func(t *ftt.Test) {
				req := plainReq()
				req.GitilesCommit = &pb.GitilesCommit{
					Host:    "gitiles.example.com",
					Project: "project",
					Ref:     "refs/heads/main",
					Id:      "1234567890123456789012345678901234567890",
				}
				_, err := schedule(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.addWithParams"`))
			})

			t.Run("template with allowed tags", func(t *ftt.Test) {
				req := templateReq()
				req.Tags = []*pb.StringPair{{Key: "user_agent", Value: "agent"}}
				_, err := schedule(ctx, req)
				assert.NoErr(t, err)
			})

			t.Run("template with other tags", func(t *ftt.Test) {
				req := templateReq()
				req.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
				_, err := schedule(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.addWithParams"`))
			})

			t.Run("template with overridden params", func(t *ftt.Test) {
				req := templateReq()
				req.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{"a": structpb.NewStringValue("c")},
				}
				_, err := schedule(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.addWithParams"`))
			})
		})

		t.Run("builds.addWithParams only", func(t *ftt.Test) {
			// builds.add is always required.
			ctx := withPerms(bbperms.BuildsAddWithParams)

			for name, req := range map[string]*pb.ScheduleBuildRequest{
				"without params":              plainReq(),
				"retry via template_build_id": templateReq(),
				"with properties":             paramsReq(),
			} {
				t.Run(name, func(t *ftt.Test) {
					_, err := schedule(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`does not have permission "buildbucket.builds.add"`))
				})
			}
		})

		t.Run("builds.add and builds.addWithParams", func(t *ftt.Test) {
			ctx := withPerms(bbperms.BuildsAdd, bbperms.BuildsAddWithParams)

			t.Run("without params", func(t *ftt.Test) {
				_, err := schedule(ctx, plainReq())
				assert.NoErr(t, err)
			})

			t.Run("retry via template_build_id", func(t *ftt.Test) {
				_, err := schedule(ctx, templateReq())
				assert.NoErr(t, err)
			})

			t.Run("with properties", func(t *ftt.Test) {
				bld, err := schedule(ctx, paramsReq())
				assert.NoErr(t, err)
				assert.Loosely(t, bld.Input.Properties.Fields["a"].GetStringValue(), should.Equal("b"))
			})

			t.Run("with other tags", func(t *ftt.Test) {
				req := plainReq()
				req.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
				_, err := schedule(ctx, req)
				assert.NoErr(t, err)
			})
		})
	})
}
