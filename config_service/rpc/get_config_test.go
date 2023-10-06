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
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey("validateGetConfig", t, func() {
		Convey("ok", func() {
			So(validateGetConfig(&pb.GetConfigRequest{
				ConfigSet:     "services/abc",
				ContentSha256: "sha256",
			}), ShouldBeNil)

			So(validateGetConfig(&pb.GetConfigRequest{
				ConfigSet: "services/abc",
				Path:      "path",
			}), ShouldBeNil)
		})

		Convey("invalid", func() {
			Convey("empty", func() {
				err := validateGetConfig(&pb.GetConfigRequest{})
				So(err, ShouldErrLike, "config_set is not specified")
			})

			Convey("path + content_sha256", func() {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet:     "services/abc",
					Path:          "path",
					ContentSha256: "hash",
				})
				So(err, ShouldErrLike, "content_sha256 and path are mutually exclusive")
			})

			Convey("config_set", func() {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
				})
				So(err, ShouldErrLike, "content_sha256 or path is required")

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "random/abc",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "random/abc": unknown domain "random" for config set "random/abc"; currently supported domains [projects, services]`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/a$c",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "services/a$c": invalid service name "a$c", expected to match "[a-z0-9\\-_]+"`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "projects/_abc",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "projects/_abc": invalid project name: must begin with a letter`)
			})

			Convey("path", func() {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "/path",
				})
				So(err, ShouldErrLike, `path "/path": must not be absolute`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "./path",
				})
				So(err, ShouldErrLike, `path "./path": should not start with './' or '../'`)
			})
		})
	})

	Convey("GetConfig", t, func() {
		ctx := testutil.SetupContext()
		srv := &Configs{}
		userID := identity.Identity("user:user@example.com")

		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ServiceAccessGroup: "service-access-group",
				ProjectAccessGroup: "project-access-group",
			},
		})
		fakeAuthDB.AddMocks(
			authtest.MockMembership(userID, "service-access-group"),
			authtest.MockMembership(userID, "project-access-group"),
		)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB:   fakeAuthDB,
		})

		Convey("not ok", func() {
			Convey("invalid req", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{ConfigSet: "services/myservice"})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "content_sha256 or path is required")
			})

			Convey("bad field mask", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
					Fields: &field_mask.FieldMask{
						Paths: []string{"random"},
					},
				})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid fields mask: field "random" does not exist in message Config`)
			})

			Convey("no permission", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity("user:random@example.com"),
					FakeDB:   fakeAuthDB,
				})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:random@example.com" does not have permission to access it`)
			})

			Convey("missing config set", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:user@example.com" does not have permission to access it`)
			})

			Convey("missing config", func() {
				testutil.InjectConfigSet(ctx, config.MustServiceSet("myservice"), map[string]proto.Message{})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:user@example.com" does not have permission to access it`)
			})

			Convey("not exist config sha256", func() {
				testutil.InjectConfigSet(ctx, config.MustServiceSet("myservice"), map[string]proto.Message{})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet:     "services/myservice",
					ContentSha256: "abc",
				})
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.NotFound, `requested resource not found or "user:user@example.com" does not have permission to access it`)
			})
		})

		Convey("ok", func() {
			file := &cfgcommonpb.ProjectCfg{Name: "file"}
			testutil.InjectConfigSet(ctx, config.MustServiceSet("myservice"), map[string]proto.Message{
				"file": file,
			})
			fileBytes, err := prototext.Marshal(file)
			So(err, ShouldBeNil)
			contentSha := sha256.Sum256(fileBytes)
			contextShaStr := hex.EncodeToString(contentSha[:])

			Convey("by configset + path", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.Config{
					ConfigSet: "services/myservice",
					Path:      "file",
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
					Revision:      "1",
					Size:          int64(len(fileBytes)),
					ContentSha256: contextShaStr,
				})
			})

			Convey("by content hash", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet:     "services/myservice",
					ContentSha256: contextShaStr,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.Config{
					ConfigSet: "services/myservice",
					Path:      "file",
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
					Revision:      "1",
					Size:          int64(len(fileBytes)),
					ContentSha256: contextShaStr,
				})
			})

			Convey("partial fields", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"config_set", "path", "content_sha256"},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.Config{
					ConfigSet:     "services/myservice",
					Path:          "file",
					ContentSha256: contextShaStr,
				})
			})

			Convey("content field mask", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"content"},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.Config{
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
				})
			})

			Convey("dup content related field masks", func() {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"content", "raw_content", "signed_url"},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.Config{
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
				})
			})

			Convey("GCS signed url", func() {
				So(datastore.Put(ctx, &model.File{
					Path:          "large",
					Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, "1"),
					ContentSHA256: "largesha256",
					GcsURI:        "gs://bucket/large",
					Location: &cfgcommonpb.Location{
						Location: &cfgcommonpb.Location_GitilesLocation{
							GitilesLocation: &cfgcommonpb.GitilesLocation{
								Repo: "repo",
								Ref:  "ref",
								Path: "path",
							},
						},
					},
				}), ShouldBeNil)

				ctl := gomock.NewController(t)
				defer ctl.Finish()
				mockGsClient := clients.NewMockGsClient(ctl)
				ctx = clients.WithGsClient(ctx, mockGsClient)

				Convey("successfully generate signed url", func() {
					mockGsClient.EXPECT().SignedURL(
						gomock.Eq("bucket"),
						gomock.Eq("large"),
						gomock.Any(),
					).Return("signed_url", nil)

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "services/myservice",
						Path:      "large",
					})
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &pb.Config{
						ConfigSet:     "services/myservice",
						Path:          "large",
						ContentSha256: "largesha256",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						Revision: "1",
						Url:      "repo/+/ref/path",
					})
				})

				Convey("failed to generate the signed url", func() {
					mockGsClient.EXPECT().SignedURL(
						gomock.Eq("bucket"),
						gomock.Eq("large"),
						gomock.Any(),
					).Return("", errors.New("GCS internal error"))

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "services/myservice",
						Path:      "large",
					})
					So(res, ShouldBeNil)
					So(err, ShouldHaveGRPCStatus, codes.Internal, "error while generating the config signed url")
				})

				Convey("content size > maxRawContentSize", func() {
					originalMaxCntSize := maxRawContentSize
					maxRawContentSize = 1
					defer func() { maxRawContentSize = originalMaxCntSize }()

					mockGsClient.EXPECT().SignedURL(
						gomock.Eq(testutil.TestGsBucket),
						gomock.Eq("file"),
						gomock.Any(),
					).Return("signed_url", nil)

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "services/myservice",
						Path:      "file",
						Fields: &field_mask.FieldMask{
							Paths: []string{"config_set", "path", "content", "revision"},
						},
					})
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &pb.Config{
						ConfigSet: "services/myservice",
						Path:      "file",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						Revision: "1",
					})
				})
			})
		})
	})
}
