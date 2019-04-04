// Copyright 2019 The LUCI Authors.
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

package utils

import (
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func performValidation(ctx context.Context, req *minter.MintProjectTokenRequest) error {
	if err := ValidateProject(ctx, req.LuciProject); err != nil {
		return err
	}
	if err := ValidateAndNormalizeRequest(ctx, req.OauthScope, &req.MinValidityDuration, req.AuditTags); err != nil {
		return err
	}
	return nil
}

func TestRpcUtils(t *testing.T) {
	ctx := gaetesting.TestingContext()

	Convey("validateRequest works", t, func() {

		Convey("empty fields", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "",
				OauthScope:          []string{},
				MinValidityDuration: 7200,
			}

			err := performValidation(ctx, req)
			So(err, ShouldNotBeNil)
		})

		Convey("empty project", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 1800,
			}
			err := performValidation(ctx, req)
			So(err, assertions.ShouldErrLike, `luci_project is empty`)
		})

		Convey("negative validity", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: -1800,
			}
			err := performValidation(ctx, req)
			So(err, assertions.ShouldErrLike, `min_validity_duration must be positive`)
		})

		Convey("normalize validity", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 0,
			}
			err := performValidation(ctx, req)
			So(err, ShouldBeNil)
			So(req.MinValidityDuration, ShouldNotEqual, 0)
		})

		Convey("malformed tags", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 0,
				AuditTags:           []string{"malformed"},
			}
			err := performValidation(ctx, req)
			So(err, ShouldNotBeNil)
		})

		Convey("empty scopes", func() {

			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{},
				MinValidityDuration: 1800,
			}

			err := performValidation(ctx, req)
			So(err, assertions.ShouldErrLike, `oauth_scope is required`)
		})

		Convey("returns nil for valid request", func() {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "test-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 3600,
			}
			err := performValidation(ctx, req)
			So(err, assertions.ShouldErrLike, "min_validity_duration must not exceed 1800")
		})
	})
}
