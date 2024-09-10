// Copyright 2021 The LUCI Authors.
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

package acls

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestCheckLegacy(t *testing.T) {
	t.Parallel()

	ftt.Run("CheckLegacyCQStatusAccess works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		cfg := &cfgpb.Config{
			CqStatusHost: "", // modified in tests below
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
			}},
		}

		t.Run("not existing project", func(t *ftt.Test) {
			allowed, err := checkLegacyCQStatusAccess(ctx, "non-existing")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowed, should.BeFalse)
		})

		t.Run("existing but disabled project", func(t *ftt.Test) {
			// Even if the previously configured CqStatusHost is public.
			cfg.CqStatusHost = validation.CQStatusHostPublic
			prjcfgtest.Create(ctx, "disabled", cfg)
			prjcfgtest.Disable(ctx, "disabled")
			allowed, err := checkLegacyCQStatusAccess(ctx, "disabled")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowed, should.BeFalse)
		})

		t.Run("without configured CQ Status", func(t *ftt.Test) {
			cfg.CqStatusHost = ""
			prjcfgtest.Create(ctx, "no-legacy", cfg)
			allowed, err := checkLegacyCQStatusAccess(ctx, "no-legacy")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowed, should.BeFalse)
		})

		t.Run("with misconfigured CQ Status", func(t *ftt.Test) {
			cfg.CqStatusHost = "misconfigured.example.com"
			prjcfgtest.Create(ctx, "misconfigured", cfg)
			allowed, err := checkLegacyCQStatusAccess(ctx, "misconfigured")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowed, should.BeFalse)
		})

		t.Run("public access", func(t *ftt.Test) {
			cfg.CqStatusHost = validation.CQStatusHostPublic
			prjcfgtest.Create(ctx, "public", cfg)
			allowed, err := checkLegacyCQStatusAccess(ctx, "public")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowed, should.BeTrue)
		})

		t.Run("internal CQ Status", func(t *ftt.Test) {
			cfg.CqStatusHost = validation.CQStatusHostInternal
			prjcfgtest.Create(ctx, "internal", cfg)

			t.Run("request by Googler is allowed", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:googler@example.com",
					IdentityGroups: []string{cqStatusInternalCrIAGroup},
				})
				allowed, err := checkLegacyCQStatusAccess(ctx, "internal")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})

			t.Run("request by non-Googler is not allowed", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:hacker@example.com",
					IdentityGroups: []string{},
				})
				allowed, err := checkLegacyCQStatusAccess(ctx, "internal")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
		})
	})
}
