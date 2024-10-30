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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(validationFile{}))
}

type mockValidator struct {
	examineResult  *validation.ExamineResult
	examineErr     error
	validateResult *cfgcommonpb.ValidationResult
	validateErr    error

	recordedExamineFiles  []validation.File
	recordedValidateFiles []validation.File
}

func (mv *mockValidator) Examine(ctx context.Context, cs config.Set, files []validation.File) (*validation.ExamineResult, error) {
	mv.recordedExamineFiles = files
	if mv.examineErr != nil {
		return nil, mv.examineErr
	}
	return mv.examineResult, nil

}

func (mv *mockValidator) Validate(ctx context.Context, cs config.Set, files []validation.File) (*cfgcommonpb.ValidationResult, error) {
	mv.recordedValidateFiles = files
	if mv.validateErr != nil {
		return nil, mv.validateErr
	}
	return mv.validateResult, nil

}

func TestValidate(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		fakeAuthDB := authtest.NewFakeDB()
		requester := identity.Identity(fmt.Sprintf("%s:requester@example.com", identity.User))
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup:     "access-group",
				ProjectValidationGroup: "validate-group",
			},
		})
		fakeAuthDB.AddMocks(
			authtest.MockMembership(requester, "access-group"),
			authtest.MockMembership(requester, "validate-group"),
		)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: requester,
			FakeDB:   fakeAuthDB,
		})

		mv := &mockValidator{}
		c := &Configs{
			Validator:          mv,
			GSValidationBucket: "test-bucket",
		}
		cs := config.MustProjectSet("example-project")
		assert.Loosely(t, datastore.Put(ctx, &model.ConfigSet{ID: cs}), should.BeNil)
		const filePath = "sub/foo.cfg"
		fileSHA256 := fmt.Sprintf("%x", sha256.Sum256([]byte("some content")))
		validateRequest := &configpb.ValidateConfigsRequest{
			ConfigSet: string(cs),
			FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
				{Path: filePath, Sha256: fileSHA256},
			},
		}

		t.Run("Invalid input", func(t *ftt.Test) {
			req := proto.Clone(validateRequest).(*configpb.ValidateConfigsRequest)
			t.Run("Empty config set", func(t *ftt.Test) {
				req.ConfigSet = ""
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("config set is required"))
			})
			t.Run("Invalid config set", func(t *ftt.Test) {
				req.ConfigSet = "bad bad"
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invalid config set"))
			})
			t.Run("Empty file hashes", func(t *ftt.Test) {
				req.FileHashes = nil
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("must provide non-empty file_hashes"))
			})
			t.Run("Empty path", func(t *ftt.Test) {
				req.FileHashes[0].Path = ""
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("file_hash[0]: path is empty"))
			})
			t.Run("Absolute path", func(t *ftt.Test) {
				req.FileHashes[0].Path = "/home/foo.cfg"
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("must not be absolute"))
			})
			for _, invalidSeg := range []string{".", ".."} {
				t.Run(fmt.Sprintf("Path contain %q", invalidSeg), func(t *ftt.Test) {
					req.FileHashes[0].Path = fmt.Sprintf("sub/%s/a.cfg", invalidSeg)
					res, err := c.ValidateConfigs(ctx, req)
					assert.Loosely(t, res, should.BeNil)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("must not contain '.' or '..' components"))
				})
			}

			t.Run("Empty hash", func(t *ftt.Test) {
				req.FileHashes[0].Sha256 = ""
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("file_hash[0]: sha256 is empty"))
			})
			t.Run("Invalid hash", func(t *ftt.Test) {
				req.FileHashes[0].Sha256 = "x.y.z"
				res, err := c.ValidateConfigs(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invalid sha256 hash"))
			})
		})

		t.Run("ACL Check", func(t *ftt.Test) {
			t.Run("Disallow anonymous", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.AnonymousIdentity,
					FakeDB:   fakeAuthDB,
				})
				res, err := c.ValidateConfigs(ctx, validateRequest)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user must be authenticated to validate config"))
			})

			t.Run("Permission Denied", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity(fmt.Sprintf("%s:another-requester@example.com", identity.User)),
					FakeDB:   fakeAuthDB,
				})
				res, err := c.ValidateConfigs(ctx, validateRequest)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("\"user:another-requester@example.com\" does not have permission to validate config set"))
			})
		})

		t.Run("Unknown config Set", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Delete(ctx, &model.ConfigSet{ID: cs}), should.BeNil)
			res, err := c.ValidateConfigs(ctx, validateRequest)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&cfgcommonpb.ValidationResult{
				Messages: []*cfgcommonpb.ValidationResult_Message{
					{
						Path:     ".",
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "The config set is not registered, skipping validation",
					},
				},
			}))
		})

		t.Run("Successful validation", func(t *ftt.Test) {
			vr := &cfgcommonpb.ValidationResult{
				Messages: []*cfgcommonpb.ValidationResult_Message{
					{
						Path:     filePath,
						Severity: cfgcommonpb.ValidationResult_ERROR,
						Text:     "something is wrong",
					},
				},
			}
			mv.examineResult = &validation.ExamineResult{} //passed
			assert.Loosely(t, mv.examineResult.Passed(), should.BeTrue)
			mv.validateResult = vr
			res, err := c.ValidateConfigs(ctx, validateRequest)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(vr))
			expectedValidationFiles := []validation.File{
				validationFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "validation", "users", "b8a0858b", "configs", "sha256", fileSHA256),
				},
			}
			assert.Loosely(t, mv.recordedExamineFiles, should.Resemble(expectedValidationFiles))
			assert.Loosely(t, mv.recordedValidateFiles, should.Resemble(expectedValidationFiles))
		})
		t.Run("Validate error", func(t *ftt.Test) {
			mv.examineResult = &validation.ExamineResult{} //passed
			mv.validateErr = errors.New("something went wrong. Transient but confidential!!!")
			res, err := c.ValidateConfigs(ctx, validateRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, err, should.ErrLike("failed to validate the configs"))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Doesn't pass examination", func(t *ftt.Test) {
			t.Run("Require uploading file", func(t *ftt.Test) {
				vf := validationFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "validation", "users", "b8a0858b", "configs", "sha256", fileSHA256),
				}
				mv.examineResult = &validation.ExamineResult{
					MissingFiles: []struct {
						File      validation.File
						SignedURL string
					}{
						{
							File:      vf,
							SignedURL: "http://example.com/signed-url",
						},
					},
				}
				mv.validateErr = errors.New("unreachable")
				res, err := c.ValidateConfigs(ctx, validateRequest)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, res, should.BeNil)
				st, ok := status.FromError(err)
				assert.Loosely(t, ok, should.BeTrue, truth.Explain("err must be a grpc error status"))
				assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, st.Message(), should.Equal("invalid validate config request. See status detail for fix instruction."))
				assert.Loosely(t, st.Details(), should.HaveLength(1))
				assert.Loosely(t, st.Details()[0], should.Resemble(&configpb.BadValidationRequestFixInfo{
					UploadFiles: []*configpb.BadValidationRequestFixInfo_UploadFile{
						{
							Path:          vf.GetPath(),
							SignedUrl:     "http://example.com/signed-url",
							MaxConfigSize: common.ConfigMaxSize,
						},
					},
				}))
				assert.Loosely(t, mv.recordedExamineFiles, should.Resemble([]validation.File{vf}))
			})
			t.Run("Unvalidatable file", func(t *ftt.Test) {
				vf := validationFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "validation", "users", "b8a0858b", "configs", "sha256", fileSHA256),
				}
				mv.examineResult = &validation.ExamineResult{
					UnvalidatableFiles: []validation.File{vf},
				}
				mv.validateErr = errors.New("unreachable")
				res, err := c.ValidateConfigs(ctx, validateRequest)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, res, should.BeNil)
				st, ok := status.FromError(err)
				assert.Loosely(t, ok, should.BeTrue, truth.Explain("err must be a grpc error status"))
				assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, st.Message(), should.Equal("invalid validate config request. See status detail for fix instruction."))
				assert.Loosely(t, st.Details(), should.HaveLength(1))
				assert.Loosely(t, st.Details()[0], should.Resemble(&configpb.BadValidationRequestFixInfo{
					UnvalidatableFiles: []string{vf.GetPath()},
				}))
				assert.Loosely(t, mv.recordedExamineFiles, should.Resemble([]validation.File{vf}))
			})
		})
	})
}
