// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"
	"testing"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
)

func createTestLUCIProject(ctx context.Context, prj, url, repo string) {
	prjcfgtest.Create(ctx, prj, &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: url,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      repo,
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
		},
	})
	meta := prjcfgtest.MustExist(ctx, prj)
	cgs, err := meta.GetConfigGroups(ctx)
	So(err, ShouldBeNil)
	So(gobmap.Update(ctx, &meta, cgs), ShouldBeNil)
}

func TestProjectFinder(t *testing.T) {
	t.Parallel()

	Convey("ProjectFinder", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		finder := &projectFinder{
			isListenerEnabled: func(string) bool { return true },
		}
		check := func(host, repo string) []string {
			prjs, err := finder.lookup(ctx, host, repo)
			So(err, ShouldBeNil)
			return prjs
		}
		createTestLUCIProject(ctx, "chromium", "https://cr-review.gs.com/", "cr/src")
		createTestLUCIProject(ctx, "chromium-m123", "https://cr-review.gs.com", "cr/src")

		lCfg := &listenerpb.Settings{
			GerritSubscriptions: []*listenerpb.Settings_GerritSubscription{
				{Host: "cr-review.gs.com"},
			},
		}

		Convey("with no disabled projects", func() {
			So(finder.reload(lCfg), ShouldBeNil)
			So(check("cr-review.gs.com", "cr/src"), ShouldResemble, []string{"chromium", "chromium-m123"})
		})

		Convey("with a disabled project", func() {
			lCfg.DisabledProjectRegexps = []string{"chromium"}
			So(finder.reload(lCfg), ShouldBeNil)
			So(check("cr-review.gs.com", "cr/src"), ShouldResemble, []string{"chromium-m123"})
		})
	})
}
