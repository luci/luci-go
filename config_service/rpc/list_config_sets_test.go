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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListConfigSets(t *testing.T) {
	t.Parallel()

	Convey("ListConfigSets", t, func() {
		ctx := testutil.SetupContext()
		srv := &Configs{}

		userID := identity.Identity("user:user@example.com")
		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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

		testutil.InjectConfigSet(ctx, config.MustProjectSet("project1"), nil)
		commitTime := clock.Now(ctx)

		Convey("invalid mask", func() {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Fields: &field_mask.FieldMask{
					Paths: []string{"file_paths"},
				},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveRPCCode, codes.InvalidArgument, `'file_paths' is not supported in fields mask`)

			res, err = srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Fields: &field_mask.FieldMask{
					Paths: []string{"random"},
				},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveRPCCode, codes.InvalidArgument, `invalid fields mask: field "random" does not exist in message ConfigSet`)
		})

		Convey("no permission", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   authtest.NewFakeDB(),
			})
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{})
		})

		Convey("partial permission", func() {
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
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			})
		})

		Convey("req for project config sets", func() {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{Domain: pb.ListConfigSetsRequest_PROJECT})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "projects/project1",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			})
		})

		Convey("req for service config sets", func() {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{Domain: pb.ListConfigSetsRequest_SERVICE})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
				ConfigSets: []*pb.ConfigSet{
					{
						Name: "services/luci-config-dev",
						Revision: &pb.ConfigSet_Revision{
							Id:        "1",
							Timestamp: timestamppb.New(commitTime),
						},
					},
				},
			})
		})

		Convey("req for all domains", func() {
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
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
			})
		})

		Convey("with last_import_attempt mask", func() {
			So(datastore.Put(ctx, &model.ImportAttempt{
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
			}), ShouldBeNil)
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Domain: pb.ListConfigSetsRequest_PROJECT,
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt", "name", "revision"},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
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
			})
		})

		Convey("with other valid masks and sub-masks", func() {
			So(datastore.Put(ctx, &model.ImportAttempt{
				ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "projects/project1"),
				Success:   true,
				Message:   "Up-to-date",
				Revision: model.RevisionInfo{
					ID:         "1",
					CommitTime: commitTime,
				},
			}), ShouldBeNil)
			res, err := srv.ListConfigSets(ctx, &pb.ListConfigSetsRequest{
				Domain: pb.ListConfigSetsRequest_PROJECT,
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt.revision.id", "name", "revision.id"},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ListConfigSetsResponse{
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
			})
		})
	})
}
