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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/swarming"
	swarmingTestdata "go.chromium.org/luci/milo/buildsource/swarming/testdata"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/testdata"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/server/templates"

	. "github.com/smartystreets/goconvey/convey"
)

type testPackage struct {
	Data         func() []common.TestBundle
	DisplayName  string
	TemplateName string
}

var (
	allPackages = []testPackage{
		{buildbotBuildTestData, "buildbot.build", "build.html"},
		{buildbotBuilderTestData, "buildbot.builder", "builder.html"},
		{consoleTestData, "console", "console.html"},
		{func() []common.TestBundle {
			return swarmingTestdata.BuildTestData(
				"../buildsource/swarming",
				func(c context.Context, svc swarmingTestdata.SwarmingService, taskID string) (*ui.MiloBuild, error) {
					build, err := swarming.SwarmingBuildImpl(c, svc, taskID)
					build.Fix()
					return build, err
				})
		}, "swarming.build", "build.html"},
		{swarmingTestdata.LogTestData, "swarming.log", "log.html"},
		{testdata.Frontpage, "frontpage", "frontpage.html"},
		{testdata.Search, "search", "search.html"},
	}
)

var generate = flag.Bool(
	"test.generate", false, "Generate expectations instead of running tests.")

func expectFileName(name string) string {
	name = strings.Replace(name, " ", "_", -1)
	name = strings.Replace(name, "/", "_", -1)
	name = strings.Replace(name, ":", "-", -1)
	return filepath.Join("expectations", name)
}

func load(name string) ([]byte, error) {
	filename := expectFileName(name)
	return ioutil.ReadFile(filename)
}

// mustWrite Writes a buffer into an expectation file.  Should always work or
// panic.  This is fine because this only runs when -generate is passed in,
// not during tests.
func mustWrite(name string, buf []byte) {
	filename := expectFileName(name)
	err := ioutil.WriteFile(filename, buf, 0644)
	if err != nil {
		panic(err)
	}
}

type analyticsSettings struct {
	AnalyticsID string `json:"analytics_id"`
}

func TestPages(t *testing.T) {
	fixZeroDurationRE := regexp.MustCompile(`(Running for:|waiting) 0s?`)
	fixZeroDuration := func(text string) string {
		return fixZeroDurationRE.ReplaceAllLiteralString(text, "[ZERO DURATION]")
	}

	Convey("Testing basic rendering.", t, func() {
		r := &http.Request{URL: &url.URL{Path: "/foobar"}}
		c := context.Background()
		c = memory.Use(c)
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
		c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
		c = settings.Use(c, settings.New(&settings.MemoryStorage{Expiration: time.Second}))
		err := settings.Set(c, "analytics", &analyticsSettings{"UA-12345-01"}, "", "")
		So(err, ShouldBeNil)
		c = templates.Use(c, getTemplateBundle("appengine/templates"), &templates.Extra{Request: r})
		for _, p := range allPackages {
			Convey(fmt.Sprintf("Testing handler %q", p.DisplayName), func() {
				for _, b := range p.Data() {
					Convey(fmt.Sprintf("Testing: %q", b.Description), func() {
						args := b.Data
						// This is not a path, but a file key, should always be "/".
						tmplName := "pages/" + p.TemplateName
						buf, err := templates.Render(c, tmplName, args)
						So(err, ShouldBeNil)
						fname := fmt.Sprintf(
							"%s-%s.html", p.DisplayName, b.Description)
						if *generate {
							mustWrite(fname, buf)
						} else {
							localBuf, err := load(fname)
							So(err, ShouldBeNil)
							So(fixZeroDuration(string(buf)), ShouldEqual, fixZeroDuration(string(localBuf)))
						}
					})
				}
			})
		}
	})
}

// buildbotBuildTestData returns sample test data for build pages.
func buildbotBuildTestData() []common.TestBundle {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	bundles := []common.TestBundle{}
	for _, tc := range buildbot.TestCases {
		build, err := buildbot.DebugBuild(c, "../buildsource/buildbot", tc.Builder, tc.Build)
		if err != nil {
			panic(fmt.Errorf(
				"Encountered error while building debug/%s/%d.\n%s",
				tc.Builder, tc.Build, err))
		}
		build.Fix()
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
	l := ui.NewLink("Some current build", "https://some.url/path", "")
	sum := &ui.BuildSummary{
		Link:     l,
		Revision: &ui.Commit{Revision: ui.NewEmptyLink("deadbeef")},
	}
	return []common.TestBundle{
		{
			Description: "Basic Test no builds",
			Data: templates.Args{
				"Builder": &ui.Builder{
					Name:         "Sample Builder",
					HasBlamelist: true,
				},
			},
		},
		{
			Description: "Basic Test with builds",
			Data: templates.Args{
				"Builder": &ui.Builder{
					Name:         "Sample Builder",
					HasBlamelist: true,
					MachinePool: &ui.MachinePool{
						Total:        15,
						Disconnected: 13,
						Idle:         5,
						Busy:         8,
					},
					CurrentBuilds:  []*ui.BuildSummary{sum},
					PendingBuilds:  []*ui.BuildSummary{sum},
					FinishedBuilds: []*ui.BuildSummary{sum},
				},
			},
		},
	}
}
