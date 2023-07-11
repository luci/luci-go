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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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

	Convey("Validate", t, func() {
		ctx := testutil.SetupContext()
		fakeAuthDB := authtest.NewFakeDB()
		requester := identity.Identity(fmt.Sprintf("%s:requester@example.com", identity.User))
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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
		So(datastore.Put(ctx, &model.ConfigSet{ID: cs}), ShouldBeNil)
		const filePath = "sub/foo.cfg"
		fileSHA256 := fmt.Sprintf("%x", sha256.Sum256([]byte("some content")))
		validateRequest := &configpb.ValidateConfigsRequest{
			ConfigSet: string(cs),
			FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
				{Path: filePath, Sha256: fileSHA256},
			},
		}

		Convey("Invalid input", func() {
			req := proto.Clone(validateRequest).(*configpb.ValidateConfigsRequest)
			Convey("Empty config set", func() {
				req.ConfigSet = ""
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "config set is required")
			})
			Convey("Invalid config set", func() {
				req.ConfigSet = "bad bad"
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "invalid config set")
			})
			Convey("Empty file hashes", func() {
				req.FileHashes = nil
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "must provide non-empty file_hashes")
			})
			Convey("Empty path", func() {
				req.FileHashes[0].Path = ""
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "file_hash[0]: path is empty")
			})
			Convey("Absolute path", func() {
				req.FileHashes[0].Path = "/home/foo.cfg"
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "must not be absolute")
			})
			for _, invalidSeg := range []string{".", ".."} {
				Convey(fmt.Sprintf("Path contain %q", invalidSeg), func() {
					req.FileHashes[0].Path = fmt.Sprintf("sub/%s/a.cfg", invalidSeg)
					res, err := c.ValidateConfigs(ctx, req)
					So(res, ShouldBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "must not contain '.' or '..' components")
				})
			}

			Convey("Empty hash", func() {
				req.FileHashes[0].Sha256 = ""
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "file_hash[0]: sha256 is empty")
			})
			Convey("Invalid hash", func() {
				req.FileHashes[0].Sha256 = "x.y.z"
				res, err := c.ValidateConfigs(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "invalid sha256 hash")
			})
		})

		Convey("ACL Check", func() {
			Convey("Disallow anonymous", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.AnonymousIdentity,
					FakeDB:   fakeAuthDB,
				})
				res, err := c.ValidateConfigs(ctx, validateRequest)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, "user must be authenticated to validate config")
			})

			Convey("Permission Denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity(fmt.Sprintf("%s:another-requester@example.com", identity.User)),
					FakeDB:   fakeAuthDB,
				})
				res, err := c.ValidateConfigs(ctx, validateRequest)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, "\"user:another-requester@example.com\" does not have permission to validate config set")
			})
		})

		Convey("Unknown config Set", func() {
			So(datastore.Delete(ctx, &model.ConfigSet{ID: cs}), ShouldBeNil)
			res, err := c.ValidateConfigs(ctx, validateRequest)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &cfgcommonpb.ValidationResult{
				Messages: []*cfgcommonpb.ValidationResult_Message{
					{
						Path:     ".",
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "The config set is not registered, skipping validation",
					},
				},
			})
		})

		Convey("Successful validation", func() {
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
			So(mv.examineResult.Passed(), ShouldBeTrue)
			mv.validateResult = vr
			res, err := c.ValidateConfigs(ctx, validateRequest)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, vr)
			expectedValidationFiles := []validation.File{
				validationFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "validation", "users", "b8a0858b", "configs", "sha256", fileSHA256),
				},
			}
			So(mv.recordedExamineFiles, ShouldResemble, expectedValidationFiles)
			So(mv.recordedValidateFiles, ShouldResemble, expectedValidationFiles)
		})
		Convey("Validate error", func() {
			mv.examineResult = &validation.ExamineResult{} //passed
			mv.validateErr = errors.New("something went wrong. Transient but confidential!!!")
			res, err := c.ValidateConfigs(ctx, validateRequest)
			So(err, ShouldHaveGRPCStatus, codes.Internal, "failed to validate the configs")
			So(res, ShouldBeNil)
		})

		Convey("Doesn't pass examination", func() {
			Convey("Require uploading file", func() {
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
				So(err, ShouldNotBeNil)
				So(res, ShouldBeNil)
				st, ok := status.FromError(err)
				SoMsg("err must be a grpc error status", ok, ShouldBeTrue)
				So(st.Code(), ShouldEqual, codes.InvalidArgument)
				So(st.Message(), ShouldEqual, "invalid validate config request. See status detail for fix instruction.")
				So(st.Details(), ShouldHaveLength, 1)
				So(st.Details()[0], ShouldResembleProto, &configpb.BadValidationRequestFixInfo{
					UploadFiles: []*configpb.BadValidationRequestFixInfo_UploadFile{
						{
							Path:          vf.GetPath(),
							SignedUrl:     "http://example.com/signed-url",
							MaxConfigSize: common.ConfigMaxSize,
						},
					},
				})
				So(mv.recordedExamineFiles, ShouldResemble, []validation.File{vf})
			})
			Convey("Unvalidatable file", func() {
				vf := validationFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "validation", "users", "b8a0858b", "configs", "sha256", fileSHA256),
				}
				mv.examineResult = &validation.ExamineResult{
					UnvalidatableFiles: []validation.File{vf},
				}
				mv.validateErr = errors.New("unreachable")
				res, err := c.ValidateConfigs(ctx, validateRequest)
				So(err, ShouldNotBeNil)
				So(res, ShouldBeNil)
				st, ok := status.FromError(err)
				SoMsg("err must be a grpc error status", ok, ShouldBeTrue)
				So(st.Code(), ShouldEqual, codes.InvalidArgument)
				So(st.Message(), ShouldEqual, "invalid validate config request. See status detail for fix instruction.")
				So(st.Details(), ShouldHaveLength, 1)
				So(st.Details()[0], ShouldResembleProto, &configpb.BadValidationRequestFixInfo{
					UnvalidatableFiles: []string{vf.GetPath()},
				})
				So(mv.recordedExamineFiles, ShouldResemble, []validation.File{vf})
			})
		})
	})
}
