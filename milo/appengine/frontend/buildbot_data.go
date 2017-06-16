// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"fmt"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/buildbot"
	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"
)

// buildbotBuildTestData returns sample test data for build pages.
func buildbotBuildTestData() []common.TestBundle {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	bundles := []common.TestBundle{}
	for _, tc := range buildbot.TestCases {
		build, err := buildbot.Build(c, "debug", tc.Builder, tc.Build)
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
						{
							Link: &resp.Link{
								URL:   "https://some.url/path",
								Label: "Some current build",
							},
							Revision: "deadbeef",
						},
					},
					PendingBuilds: []*resp.BuildSummary{
						{
							Link: &resp.Link{
								URL:   "https://some.url/path",
								Label: "Some current build",
							},
							Revision: "deadbeef",
						},
					},
					FinishedBuilds: []*resp.BuildSummary{
						{
							Link: &resp.Link{
								URL:   "https://some.url/path",
								Label: "Some current build",
							},
							Revision: "deadbeef",
						},
					},
				},
			},
		},
	}
}
