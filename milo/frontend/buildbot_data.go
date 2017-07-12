// Copyright 2016 The LUCI Authors.
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

package frontend

import (
	"fmt"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/buildsource/buildbot"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"
)

// buildbotBuildTestData returns sample test data for build pages.
func buildbotBuildTestData() []common.TestBundle {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	bundles := []common.TestBundle{}
	for _, tc := range buildbot.TestCases {
		build, err := buildbot.DebugBuild(c, "../buildsource/buildbot", tc.Builder, tc.Build)
		if err != nil {
			panic(fmt.Errorf(
				"Encountered error while building debug/%s/%s.\n%s",
				tc.Builder, tc.Build, err))
		}
		bundles = append(bundles, common.TestBundle{
			Description: fmt.Sprintf("Debug page: %s/%d", tc.Builder, tc.Build),
			Data: templates.Args{
				"Build": build,
			},
		})
	}
	return bundles
}

// buildbotBuilderTestData returns sample test data for builder pages.
func buildbotBuilderTestData() []common.TestBundle {
	l := resp.NewLink("Some current build", "https://some.url/path")
	return []common.TestBundle{
		{
			Description: "Basic Test no builds",
			Data: templates.Args{
				"Builder": &resp.Builder{
					Name: "Sample Builder",
				},
			},
		},
		{
			Description: "Basic Test with builds",
			Data: templates.Args{
				"Builder": &resp.Builder{
					Name: "Sample Builder",
					MachinePool: &resp.MachinePool{
						Total:        15,
						Disconnected: 13,
						Idle:         5,
						Busy:         8,
					},
					CurrentBuilds: []*resp.BuildSummary{
						{Link: l, Revision: "deadbeef"},
					},
					PendingBuilds: []*resp.BuildSummary{
						{Link: l, Revision: "deadbeef"},
					},
					FinishedBuilds: []*resp.BuildSummary{
						{Link: l, Revision: "deadbeef"},
					},
				},
			},
		},
	}
}
