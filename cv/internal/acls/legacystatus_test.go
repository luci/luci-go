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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckLegacy(t *testing.T) {
	t.Parallel()

	Convey("CheckLegacyCQStatusAccess works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		cfg := &cfgpb.Config{
			CqStatusHost: "", // modified in tests below
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
			}},
		}

		Convey("not existing project", func() {
			allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("non-existing/123-1-cafe")})
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("existing but disabled project", func() {
			// Even if the previously configured CqStatusHost is public.
			cfg.CqStatusHost = cqStatusHostPublic
			prjcfgtest.Create(ctx, "disabled", cfg)
			prjcfgtest.Disable(ctx, "disabled")
			allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("disabled/123-1-cafe")})
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("without configured CQ Status", func() {
			cfg.CqStatusHost = ""
			prjcfgtest.Create(ctx, "no-legacy", cfg)
			allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("no-legacy/123-1-cafe")})
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("with misconfigured CQ Status", func() {
			cfg.CqStatusHost = "misconfigured.example.com"
			prjcfgtest.Create(ctx, "misconfigured", cfg)
			allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("misconfigured/123-1-cafe")})
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("public access", func() {
			cfg.CqStatusHost = cqStatusHostPublic
			prjcfgtest.Create(ctx, "public", cfg)
			allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("public/123-1-cafe")})
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)
		})

		Convey("internal CQ Status", func() {
			cfg.CqStatusHost = cqStatusHostInternal
			prjcfgtest.Create(ctx, "internal", cfg)

			Convey("request by Googler is allowed", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:googler@example.com",
					IdentityGroups: []string{cqStatusInternalCrIAGroup},
				})
				allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("internal/123-1-cafe")})
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})

			Convey("request by non-Googler is not allowed", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:hacker@example.com",
					IdentityGroups: []string{},
				})
				allowed, err := CheckLegacyCQStatusAccess(ctx, &run.Run{ID: common.RunID("internal/123-1-cafe")})
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
		})
	})
}
