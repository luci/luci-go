// Copyright 2023 The LUCI Authors.
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
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"
)

// sha256("")
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestGetConfigSet(t *testing.T) {
	t.Parallel()

	ftt.Run("GetConfigSet", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		srv := &Configs{}

		userID := identity.Identity("user:user@example.com")
		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup: "project-access-group",
			},
		})
		fakeAuthDB.AddMocks(
			authtest.MockMembership(userID, "project-access-group"),
		)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB:   fakeAuthDB,
		})

		testutil.InjectConfigSet(ctx, t, config.MustProjectSet("project"), map[string]proto.Message{
			"config1.cfg": nil,
			"config2.cfg": nil,
		})
		commitTime := clock.Now(ctx)
		assert.Loosely(t, datastore.Put(ctx, &model.ImportAttempt{
			ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "projects/project"),
			Success:   true,
			Message:   "Up-to-date",
			Revision: model.RevisionInfo{
				ID:             "1",
				CommitTime:     commitTime,
				CommitterEmail: "committer@gmail.com",
				AuthorEmail:    "author@gmail.com",
			},
			ValidationResult: &cfgcommonpb.ValidationResult{
				Messages: []*cfgcommonpb.ValidationResult_Message{
					{
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "There is a warning",
					},
				},
			},
		}), should.BeNil)

		t.Run("empty req", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("config_set is not specified"))
		})

		t.Run("invalid config set", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "project/abc",
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`unknown domain "project" for config set "project/abc"; currently supported domains [projects, services]`))
		})

		t.Run("invalid field mask", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"random"},
				},
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invalid fields mask: field "random" does not exist in message ConfigSet`))
		})

		t.Run("no permission", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   authtest.NewFakeDB(),
			})
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:random@example.com" does not have permission to access it`))
		})

		t.Run("non-existent configSet", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/random",
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:user@example.com" does not have permission to access it`))
		})

		t.Run("ok with default mask", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
			}))
		})

		t.Run("with last_import_attempt", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt", "name", "revision"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
				LastImportAttempt: &pb.ConfigSet_Attempt{
					Message: "Up-to-date",
					Success: true,
					Revision: &pb.ConfigSet_Revision{
						Id:             "1",
						Timestamp:      timestamppb.New(commitTime),
						CommitterEmail: "committer@gmail.com",
						AuthorEmail:    "author@gmail.com",
					},
					ValidationResult: &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Severity: cfgcommonpb.ValidationResult_WARNING,
								Text:     "There is a warning",
							},
						},
					},
				},
			}))
		})

		t.Run("with file_paths", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"file_paths", "name", "revision"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
				FilePaths: []string{"config1.cfg", "config2.cfg"},
			}))
		})

		t.Run("with configs", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"configs", "name", "revision"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
				Configs: []*pb.Config{
					{ConfigSet: "projects/project", Path: "config1.cfg", Revision: "1", ContentSha256: emptySHA256},
					{ConfigSet: "projects/project", Path: "config2.cfg", Revision: "1", ContentSha256: emptySHA256},
				},
			}))
		})

		t.Run("with mixed masks and sub-masks", func(t *ftt.Test) {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{
						"configs",
						"name",
						"revision.id",
						"last_import_attempt.success",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id: "1",
				},
				Configs: []*pb.Config{
					{ConfigSet: "projects/project", Path: "config1.cfg", Revision: "1", ContentSha256: emptySHA256},
					{ConfigSet: "projects/project", Path: "config2.cfg", Revision: "1", ContentSha256: emptySHA256},
				},
				LastImportAttempt: &pb.ConfigSet_Attempt{
					Success: true,
				},
			}))
		})
	})
}
