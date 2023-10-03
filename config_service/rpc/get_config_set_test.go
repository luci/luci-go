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

func TestGetConfigSet(t *testing.T) {
	t.Parallel()

	Convey("GetConfigSet", t, func() {
		ctx := testutil.SetupContext()
		srv := &Configs{}

		userID := identity.Identity("user:user@example.com")
		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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

		testutil.InjectConfigSet(ctx, config.MustProjectSet("project"), map[string]proto.Message{
			"config1.cfg": nil,
			"config2.cfg": nil,
		})
		commitTime := clock.Now(ctx)
		So(datastore.Put(ctx, &model.ImportAttempt{
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
		}), ShouldBeNil)

		Convey("empty req", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "config_set is not specified")
		})

		Convey("invalid config set", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "project/abc",
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `unknown domain "project" for config set "project/abc"; currently supported domains [projects, services]`)
		})

		Convey("invalid field mask", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"random"},
				},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid fields mask: field "random" does not exist in message ConfigSet`)
		})

		Convey("no permission", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   authtest.NewFakeDB(),
			})
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:random@example.com" does not have permission to access it`)
		})

		Convey("non-existent configSet", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/random",
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:user@example.com" does not have permission to access it`)
		})

		Convey("ok with default mask", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
			})
		})

		Convey("with last_import_attempt", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"last_import_attempt", "name", "revision"},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ConfigSet{
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
			})
		})

		Convey("with file_paths", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"file_paths", "name", "revision"},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id:        "1",
					Timestamp: timestamppb.New(commitTime),
				},
				FilePaths: []string{"config1.cfg", "config2.cfg"},
			})
		})

		Convey("with mixed masks and sub-masks", func() {
			res, err := srv.GetConfigSet(ctx, &pb.GetConfigSetRequest{
				ConfigSet: "projects/project",
				Fields: &field_mask.FieldMask{
					Paths: []string{"file_paths", "name", "revision.id", "last_import_attempt.success"},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.ConfigSet{
				Name: "projects/project",
				Revision: &pb.ConfigSet_Revision{
					Id: "1",
				},
				FilePaths: []string{"config1.cfg", "config2.cfg"},
				LastImportAttempt: &pb.ConfigSet_Attempt{
					Success: true,
				},
			})
		})
	})
}
