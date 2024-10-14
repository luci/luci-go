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
	"context"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
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

	ftt.Run("validateRequest works", t, func(t *ftt.Test) {

		t.Run("empty fields", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "",
				OauthScope:          []string{},
				MinValidityDuration: 7200,
			}

			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("empty project", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 1800,
			}
			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`luci_project is empty`))
		})

		t.Run("negative validity", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: -1800,
			}
			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`min_validity_duration must be positive`))
		})

		t.Run("normalize validity", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 0,
			}
			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req.MinValidityDuration, should.NotEqual(0))
		})

		t.Run("malformed tags", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 0,
				AuditTags:           []string{"malformed"},
			}
			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("empty scopes", func(t *ftt.Test) {

			req := &minter.MintProjectTokenRequest{
				LuciProject:         "foo-project",
				OauthScope:          []string{},
				MinValidityDuration: 1800,
			}

			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`oauth_scope is required`))
		})

		t.Run("returns nil for valid request", func(t *ftt.Test) {
			req := &minter.MintProjectTokenRequest{
				LuciProject:         "test-project",
				OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				MinValidityDuration: 3600,
			}
			err := performValidation(ctx, req)
			assert.Loosely(t, err, should.ErrLike("min_validity_duration must not exceed 1800"))
		})
	})
}
