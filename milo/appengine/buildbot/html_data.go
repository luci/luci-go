// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"fmt"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"
)

// We put this here because _test.go files are sometimes not built.
var testCases = []struct {
	builder string
	build   int
}{
	{"CrWinGoma", 30608},
	{"win_chromium_rel_ng", 246309},
}

// BuildTestData returns sample test data for build pages.
func BuildTestData() []common.TestBundle {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	bundles := []common.TestBundle{}
	for _, tc := range testCases {
		build, err := build(c, "debug", tc.builder, tc.build)
		if err != nil {
			panic(fmt.Errorf(
				"Encountered error while building debug/%s/%s.\n%s",
				tc.builder, tc.build, err))
		}
		bundles = append(bundles, common.TestBundle{
			Description: fmt.Sprintf("Debug page: %s/%d", tc.builder, tc.build),
			Data: templates.Args{
				"Build": build,
			},
		})
	}
	return bundles
}

// BiulderTestData returns sample test data for builder pages.
func BuilderTestData() []common.TestBundle {
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
