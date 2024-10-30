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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("validateGetConfig", t, func(t *ftt.Test) {
		t.Run("ok", func(t *ftt.Test) {
			assert.Loosely(t, validateGetConfig(&pb.GetConfigRequest{
				ConfigSet:     "services/abc",
				ContentSha256: "sha256",
			}), should.BeNil)

			assert.Loosely(t, validateGetConfig(&pb.GetConfigRequest{
				ConfigSet: "services/abc",
				Path:      "path",
			}), should.BeNil)
		})

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				err := validateGetConfig(&pb.GetConfigRequest{})
				assert.Loosely(t, err, should.ErrLike("config_set is not specified"))
			})

			t.Run("path + content_sha256", func(t *ftt.Test) {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet:     "services/abc",
					Path:          "path",
					ContentSha256: "hash",
				})
				assert.Loosely(t, err, should.ErrLike("content_sha256 and path are mutually exclusive"))
			})

			t.Run("config_set", func(t *ftt.Test) {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
				})
				assert.Loosely(t, err, should.ErrLike("content_sha256 or path is required"))

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "random/abc",
					Path:      "path",
				})
				assert.Loosely(t, err, should.ErrLike(`config_set "random/abc": unknown domain "random" for config set "random/abc"; currently supported domains [projects, services]`))

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/a$c",
					Path:      "path",
				})
				assert.Loosely(t, err, should.ErrLike(`config_set "services/a$c": invalid service name "a$c", expected to match "[a-z0-9\\-_]+"`))

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "projects/_abc",
					Path:      "path",
				})
				assert.Loosely(t, err, should.ErrLike(`config_set "projects/_abc": invalid project name: must begin with a letter`))
			})

			t.Run("path", func(t *ftt.Test) {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "/path",
				})
				assert.Loosely(t, err, should.ErrLike(`path "/path": must not be absolute`))

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "./path",
				})
				assert.Loosely(t, err, should.ErrLike(`path "./path": should not start with './' or '../'`))
			})
		})
	})

	ftt.Run("GetConfig", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		srv := &Configs{}
		userID := identity.Identity("user:user@example.com")

		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
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

		t.Run("not ok", func(t *ftt.Test) {
			t.Run("invalid req", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{ConfigSet: "services/myservice"})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("content_sha256 or path is required"))
			})

			t.Run("bad field mask", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
					Fields: &field_mask.FieldMask{
						Paths: []string{"random"},
					},
				})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid fields mask: field "random" does not exist in message Config`))
			})

			t.Run("no permission", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity("user:random@example.com"),
					FakeDB:   fakeAuthDB,
				})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:random@example.com" does not have permission to access it`))
			})

			t.Run("missing config set", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:user@example.com" does not have permission to access it`))
			})

			t.Run("missing config", func(t *ftt.Test) {
				testutil.InjectConfigSet(ctx, t, config.MustServiceSet("myservice"), map[string]proto.Message{})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "path",
				})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:user@example.com" does not have permission to access it`))
			})

			t.Run("not exist config sha256", func(t *ftt.Test) {
				testutil.InjectConfigSet(ctx, t, config.MustServiceSet("myservice"), map[string]proto.Message{})
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet:     "services/myservice",
					ContentSha256: "abc",
				})
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`requested resource not found or "user:user@example.com" does not have permission to access it`))
			})
		})

		t.Run("ok", func(t *ftt.Test) {
			file := &cfgcommonpb.ProjectCfg{Name: "file"}
			testutil.InjectConfigSet(ctx, t, config.MustServiceSet("myservice"), map[string]proto.Message{
				"file": file,
			})
			fileBytes, err := prototext.Marshal(file)
			assert.Loosely(t, err, should.BeNil)
			contentSha := sha256.Sum256(fileBytes)
			contextShaStr := hex.EncodeToString(contentSha[:])

			t.Run("by configset + path", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.Config{
					ConfigSet: "services/myservice",
					Path:      "file",
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
					Revision:      "1",
					Size:          int64(len(fileBytes)),
					ContentSha256: contextShaStr,
				}))
			})

			t.Run("by content hash", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet:     "services/myservice",
					ContentSha256: contextShaStr,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.Config{
					ConfigSet: "services/myservice",
					Path:      "file",
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
					Revision:      "1",
					Size:          int64(len(fileBytes)),
					ContentSha256: contextShaStr,
				}))
			})

			t.Run("partial fields", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"config_set", "path", "content_sha256"},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.Config{
					ConfigSet:     "services/myservice",
					Path:          "file",
					ContentSha256: contextShaStr,
				}))
			})

			t.Run("content field mask", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"content"},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.Config{
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
				}))
			})

			t.Run("dup content related field masks", func(t *ftt.Test) {
				res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
					ConfigSet: "services/myservice",
					Path:      "file",
					Fields: &field_mask.FieldMask{
						Paths: []string{"content", "raw_content", "signed_url"},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.Config{
					Content: &pb.Config_RawContent{
						RawContent: fileBytes,
					},
				}))
			})

			t.Run("GCS signed url", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.File{
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
				}), should.BeNil)

				ctl := gomock.NewController(t)
				defer ctl.Finish()
				mockGsClient := clients.NewMockGsClient(ctl)
				ctx = clients.WithGsClient(ctx, mockGsClient)

				t.Run("successfully generate signed url", func(t *ftt.Test) {
					mockGsClient.EXPECT().SignedURL(
						gomock.Eq("bucket"),
						gomock.Eq("large"),
						gomock.Any(),
					).Return("signed_url", nil)

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "services/myservice",
						Path:      "large",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.Config{
						ConfigSet:     "services/myservice",
						Path:          "large",
						ContentSha256: "largesha256",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						Revision: "1",
						Url:      "repo/+/ref/path",
					}))
				})

				t.Run("failed to generate the signed url", func(t *ftt.Test) {
					mockGsClient.EXPECT().SignedURL(
						gomock.Eq("bucket"),
						gomock.Eq("large"),
						gomock.Any(),
					).Return("", errors.New("GCS internal error"))

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "services/myservice",
						Path:      "large",
					})
					assert.Loosely(t, res, should.BeNil)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
					assert.Loosely(t, err, should.ErrLike("error while generating the config signed url"))
				})

				t.Run("content size > maxRawContentSize", func(t *ftt.Test) {
					fooPb := &cfgcommonpb.ProjectCfg{Name: strings.Repeat("0123456789", maxRawContentSize/10)}
					assert.Loosely(t, err, should.BeNil)
					testutil.InjectConfigSet(ctx, t, config.MustProjectSet("foo"), map[string]proto.Message{
						"foo.cfg": fooPb,
					})

					mockGsClient.EXPECT().SignedURL(
						gomock.Eq(testutil.TestGsBucket),
						gomock.Eq("foo.cfg"),
						gomock.Any(),
					).Return("signed_url", nil)

					res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{
						ConfigSet: "projects/foo",
						Path:      "foo.cfg",
						Fields: &field_mask.FieldMask{
							Paths: []string{"config_set", "path", "content", "revision"},
						},
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.Config{
						ConfigSet: "projects/foo",
						Path:      "foo.cfg",
						Content: &pb.Config_SignedUrl{
							SignedUrl: "signed_url",
						},
						Revision: "1",
					}))
				})
			})
		})
	})
}
