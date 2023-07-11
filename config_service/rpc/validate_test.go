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
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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

		c := &Configs{}
		cs := config.MustProjectSet("example-project")
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

	})
}
