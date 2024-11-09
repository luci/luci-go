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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"
)

func TestGetProjectConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("GetProjectConfigs", t, func(t *ftt.Test) {
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

		// Inject "services/myservice"
		testutil.InjectConfigSet(ctx, t, config.MustServiceSet("myservice"), nil)

		// Inject "projects/project1" with a small "config.cfg" file and "other1.cfg" file.
		configPb := &cfgcommonpb.ProjectCfg{Name: "config.cfg"}
		configPbBytes, err := prototext.Marshal(configPb)
		assert.Loosely(t, err, should.BeNil)
		configPbSha := sha256.Sum256(configPbBytes)
		configPbShaStr := hex.EncodeToString(configPbSha[:])
		testutil.InjectConfigSet(ctx, t, config.MustProjectSet("project1"), map[string]proto.Message{
			"config.cfg": configPb,
			"other1.cfg": &cfgcommonpb.ProjectCfg{Name: "other1.cfg"},
		})

		// Inject "projects/project2" with a large "config.cfg" file and "other2.cfg" file.
		testutil.InjectConfigSet(ctx, t, config.MustProjectSet("project2"), map[string]proto.Message{
			"other2.cfg": &cfgcommonpb.ProjectCfg{Name: "other2.cfg"},
		})
		assert.Loosely(t, datastore.Put(ctx, &model.File{
			Path:          "config.cfg",
			Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "projects/project2", model.RevisionKind, "1"),
			ContentSHA256: "configsha256",
			GcsURI:        "gs://bucket/configsha256",
			Size:          1000,
		}), should.BeNil)
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockGsClient := clients.NewMockGsClient(ctl)
		ctx = clients.WithGsClient(ctx, mockGsClient)

		t.Run("invalid path", func(t *ftt.Test) {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invalid path - "": not specified`))

			res, err = srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{Path: "/file"})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invalid path - "/file": must not be absolute`))

			res, err = srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{Path: "./file"})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invalid path - "./file": should not start with './' or '../'`))
		})

		t.Run("invalid mask", func(t *ftt.Test) {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "file",
				Fields: &field_mask.FieldMask{
					Paths: []string{"random"},
				},
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invalid fields mask: field "random" does not exist in message Config`))
		})

		t.Run("no access to matched files", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   fakeAuthDB,
			})
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "config.cfg",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{}))
		})

		t.Run("no matched files", func(t *ftt.Test) {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "non_exist.cfg",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{}))
		})

		t.Run("found", func(t *ftt.Test) {
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq("bucket"),
				gomock.Eq("configsha256"),
				gomock.Any(),
			).Return("signed_url", nil)

			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "config.cfg",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{
				Configs: []*pb.Config{
					{
						ConfigSet: "projects/project1",
						Path:      "config.cfg",
						Content: &pb.Config_RawContent{
							RawContent: configPbBytes,
						},
						ContentSha256: configPbShaStr,
						Revision:      "1",
						Size:          int64(len(configPbBytes)),
					},
					{
						ConfigSet: "projects/project2",
						Path:      "config.cfg",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						ContentSha256: "configsha256",
						Revision:      "1",
						Size:          1000,
					},
				},
			}))
		})

		t.Run(" config size > maxRawContentSize", func(t *ftt.Test) {
			fooPb := &cfgcommonpb.ProjectCfg{Name: strings.Repeat("0123456789", maxRawContentSize/10)}
			fooPbBytes, err := prototext.Marshal(fooPb)
			assert.Loosely(t, err, should.BeNil)
			fooPbSha := sha256.Sum256(fooPbBytes)
			fooPbShaStr := hex.EncodeToString(fooPbSha[:])

			testutil.InjectConfigSet(ctx, t, config.MustProjectSet("foo"), map[string]proto.Message{
				"foo.cfg": fooPb,
			})
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq(testutil.TestGsBucket),
				gomock.Eq("foo.cfg"),
				gomock.Any(),
			).Return("signed_url", nil)

			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "foo.cfg",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{
				Configs: []*pb.Config{
					{
						ConfigSet: "projects/foo",
						Path:      "foo.cfg",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						ContentSha256: fooPbShaStr,
						Revision:      "1",
						Size:          int64(len(fooPbBytes)),
					},
				},
			}))
		})

		t.Run("total size > maxProjConfigsResSize", func(t *ftt.Test) {
			originalLimit := maxProjConfigsResSize
			// Make the limit to 1 byte to avoid taking too much memory to test this
			// use case.
			maxProjConfigsResSize = 1
			defer func() { maxProjConfigsResSize = originalLimit }()

			mockGsClient.EXPECT().SignedURL(
				gomock.Eq(testutil.TestGsBucket),
				gomock.Eq("config.cfg"),
				gomock.Any(),
			).Return("signed_url1", nil)
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq("bucket"),
				gomock.Eq("configsha256"),
				gomock.Any(),
			).Return("signed_url2", nil)

			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "config.cfg",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{
				Configs: []*pb.Config{
					{
						ConfigSet: "projects/project1",
						Path:      "config.cfg",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url1",
						},
						ContentSha256: configPbShaStr,
						Revision:      "1",
						Size:          int64(len(configPbBytes)),
					},
					{
						ConfigSet: "projects/project2",
						Path:      "config.cfg",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url2",
						},
						ContentSha256: "configsha256",
						Revision:      "1",
						Size:          1000,
					},
				},
			}))
		})

		t.Run("mask", func(t *ftt.Test) {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "other1.cfg",
				Fields: &field_mask.FieldMask{
					Paths: []string{"config_set", "path"},
				},
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.GetProjectConfigsResponse{
				Configs: []*pb.Config{
					{
						ConfigSet: "projects/project1",
						Path:      "other1.cfg",
					},
				},
			}))
		})

		t.Run("GCS error on signed url", func(t *ftt.Test) {
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq("bucket"),
				gomock.Eq("configsha256"),
				gomock.Any(),
			).Return("", errors.New("GCS internal error"))

			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "config.cfg",
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, err, should.ErrLike("error while generating the config signed url"))
		})
	})
}
