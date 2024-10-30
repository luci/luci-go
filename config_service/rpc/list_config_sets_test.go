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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestListConfigSets(t *testing.T) {
	t.Parallel()

	ftt.Run("ListConfigSets", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		srv := &Configs{}

		userID := identity.Identity("user:user@example.com")
		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup: "project-access-group",
				ServiceAccessGroup: "service-access-group",
			},
		})
		fakeAuthDB.AddMocks(
			authtest.MockMembership(userID, "project-access-group"),
			authtest.MockMembership(userID, "service-access-group"),
		)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB:   fakeAuthDB,
		})

		testutil.InjectConfigSet(ctx, t, config.MustProjectSet("project1"), nil)
		commitTime := clock.Now(ctx)

		t.Run("invalid mask", func(t *ftt.Test) {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Fields: &field_mask.FieldMask{
					Paths: []string{"configs"},
				},
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`'configs' is not supported in fields mask`))

			res, err = srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
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
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{}))
		})

		t.Run("partial permission", func(t *ftt.Test) {
			another := identity.Identity("user:another@example.com")
			fakeAuthDB1 := authtest.NewFakeDB()
			fakeAuthDB1.AddMocks(
				authtest.MockMembership(another, "project-access-group"),
			)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: another,
				FakeDB:   fakeAuthDB1,
			})
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			}))
		})

		t.Run("req for project config sets", func(t *ftt.Test) {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{Domain: pb.ListConfigSetsRequest_PROJECT})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			}))
		})

		t.Run("req for service config sets", func(t *ftt.Test) {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{Domain: pb.ListConfigSetsRequest_SERVICE})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "services/luci-config-dev",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			}))
		})

		t.Run("req for all domains", func(t *ftt.Test) {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
					{
						Name: "services/luci-config-dev",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			}))
		})

		t.Run("with last_import_attempt mask", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.ImportAttempt{
				ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "projects/project1"),
				Success:   true,
				Message:   "Up-to-date",
				Revision: model.RevisionInfo{
					ID:         "1",
					CommitTime: commitTime,
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
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Domain: pb.ListConfigSetsRequest_PROJECT,
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt", "name", "revision"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
						LastImportAttempt: &pb.ConfigSet_Attempt{
							Message: "Up-to-date",
							Success: true,
							Revision: &pb.ConfigSet_Revision{
								Id:        "1",
								Timestamp: timestamppb.New(commitTime),
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
					},
				},
			}))
		})

		t.Run("with other valid masks and sub-masks", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.ImportAttempt{
				ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "projects/project1"),
				Success:   true,
				Message:   "Up-to-date",
				Revision: model.RevisionInfo{
					ID:         "1",
					CommitTime: commitTime,
				},
			}), should.BeNil)
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Domain: pb.ListConfigSetsRequest_PROJECT,
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt.revision.id", "name", "revision.id"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id: "1",
						},
						LastImportAttempt: &pb.ConfigSet_Attempt{
							Revision: &pb.ConfigSet_Revision{
								Id: "1",
							},
						},
					},
				},
			}))
		})
	})
}
