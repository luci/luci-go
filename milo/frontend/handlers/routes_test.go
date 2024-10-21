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

package handlers

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/julienschmidt/httprouter"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
)

var now = time.Date(2019, time.February, 3, 4, 5, 6, 7, time.UTC)
var nowTS = timestamppb.New(now)

// TODO(nodir): refactor this file.

// TestBundle is a template arg associated with a description used for testing.
type TestBundle struct {
	// Description is a short one line description of what the data contains.
	Description string
	// Data is the data fed directly into the template.
	Data templates.Args
}

type testPackage struct {
	Data         func() []TestBundle
	DisplayName  string
	TemplateName string
}

var (
	allPackages = []testPackage{
		{buildbucketBuildTestData, "buildbucket.build", "pages/build.html"},
		{consoleTestData, "console", "pages/console.html"},
		{Frontpage, "frontpage", "pages/frontpage.html"},
		{relatedBuildsTableTestData, "widget", "widgets/related_builds_table.html"},
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
	return os.ReadFile(filename)
}

// mustWrite Writes a buffer into an expectation file.  Should always work or
// panic.  This is fine because this only runs when -generate is passed in,
// not during tests.
func mustWrite(name string, buf []byte) {
	filename := expectFileName(name)
	err := os.WriteFile(filename, buf, 0644)
	if err != nil {
		panic(err)
	}
}

func TestPages(t *testing.T) {
	fixZeroDurationRE := regexp.MustCompile(`(Running for:|waiting) 0s?`)
	fixZeroDuration := func(text string) string {
		return fixZeroDurationRE.ReplaceAllLiteralString(text, "[ZERO DURATION]")
	}

	ftt.Run("Testing basic rendering.", t, func(t *ftt.Test) {
		t.Skip("Accidentally skipped in https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3216129")

		r := &http.Request{URL: &url.URL{Path: "/foobar"}}
		c := context.Background()
		c = memory.Use(c)
		c, _ = testclock.UseTime(c, now)
		c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
		c = settings.Use(c, settings.New(&settings.MemoryStorage{Expiration: time.Second}))
		c = templates.Use(c, getTemplateBundle("appengine/templates", "testVersionID", false), &templates.Extra{Request: r})
		for _, p := range allPackages {
			t.Run(fmt.Sprintf("Testing handler %q", p.DisplayName), func(t *ftt.Test) {
				for _, b := range p.Data() {
					t.Run(fmt.Sprintf("Testing: %q", b.Description), func(t *ftt.Test) {
						args := b.Data
						// This is not a path, but a file key, should always be "/".
						tmplName := p.TemplateName
						buf, err := templates.Render(c, tmplName, args)
						assert.Loosely(t, err, should.BeNil)
						fname := fmt.Sprintf(
							"%s-%s.html", p.DisplayName, b.Description)
						if *generate {
							mustWrite(fname, buf)
						} else {
							localBuf, err := load(fname)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, fixZeroDuration(string(buf)), should.Equal(fixZeroDuration(string(localBuf))))
						}
					})
				}
			})
		}
	})
}

// buildbucketBuildTestData returns sample test data for build pages.
func buildbucketBuildTestData() []TestBundle {
	bundles := []TestBundle{}
	for _, tc := range []string{"linux-rel", "MacTests", "scheduled"} {
		build, err := GetTestBuild("../buildsource/buildbucket", tc)
		if err != nil {
			panic(fmt.Errorf("Encountered error while fetching %s.\n%s", tc, err))
		}
		bundles = append(bundles, TestBundle{
			Description: fmt.Sprintf("Test page: %s", tc),
			Data: templates.Args{
				"BuildPage": &ui.BuildPage{
					Build: ui.Build{
						Build: build,
						Now:   nowTS,
					},
					BuildbucketHost: "example.com",
				},
				"XsrfTokenField": template.HTML(`<input name="[XSRF Token]" type="hidden" value="[XSRF Token]">`),
				"RetryRequestID": "[Retry Request ID]",
			},
		})
	}
	return bundles
}

func consoleTestData() []TestBundle {
	builder := &ui.BuilderRef{
		ID:        "buildbucket/luci.project-foo.try/builder-bar",
		ShortName: "tst",
		Build: []*model.BuildSummary{
			{
				Summary: model.Summary{
					Status: milostatus.Success,
				},
			},
			nil,
		},
		Builder: &model.BuilderSummary{
			BuilderID:          "buildbucket/luci.project-foo.try/builder-bar",
			ProjectID:          "project-foo",
			LastFinishedStatus: milostatus.InfraFailure,
		},
	}
	root := ui.NewCategory("Root")
	root.AddBuilder([]string{"cat1", "cat2"}, builder)
	return []TestBundle{
		{
			Description: "Full console with Header",
			Data: templates.Args{
				"Expand": false,
				"Console": consoleRenderer{&ui.Console{
					Name:    "Test",
					Project: "Testing",
					Header: &ui.ConsoleHeader{
						Oncalls: []*ui.OncallSummary{
							{
								Name:      "Sheriff",
								Oncallers: template.HTML("test (primary), watcher (secondary)"),
							},
						},
						Links: []ui.LinkGroup{
							{
								Name: ui.NewLink("Some group", "", ""),
								Links: []*ui.Link{
									ui.NewLink("LiNk", "something", ""),
									ui.NewLink("LiNk2", "something2", ""),
								},
							},
						},
						ConsoleGroups: []ui.ConsoleGroup{
							{
								Title: ui.NewLink("bah", "something2", ""),
								Consoles: []*ui.BuilderSummaryGroup{
									{
										Name: ui.NewLink("hurrah", "something2", ""),
										Builders: []*model.BuilderSummary{
											{
												LastFinishedStatus: milostatus.Success,
											},
											{
												LastFinishedStatus: milostatus.Success,
											},
											{
												LastFinishedStatus: milostatus.Failure,
											},
										},
									},
								},
							},
							{
								Consoles: []*ui.BuilderSummaryGroup{
									{
										Name: ui.NewLink("hurrah", "something2", ""),
										Builders: []*model.BuilderSummary{
											{
												LastFinishedStatus: milostatus.Success,
											},
										},
									},
								},
							},
						},
					},
					Commit: []ui.Commit{
						{
							AuthorEmail: "x@example.com",
							CommitTime:  time.Date(12, 12, 12, 12, 12, 12, 0, time.UTC),
							Revision:    ui.NewLink("12031802913871659324", "blah blah blah", ""),
							Description: "Me too.",
						},
						{
							AuthorEmail: "y@example.com",
							CommitTime:  time.Date(12, 12, 12, 12, 12, 11, 0, time.UTC),
							Revision:    ui.NewLink("120931820931802913", "blah blah blah 1", ""),
							Description: "I did something.",
						},
					},
					Table:    *root,
					MaxDepth: 3,
				}},
			},
		},
	}
}

func Frontpage() []TestBundle {
	return []TestBundle{
		{
			Description: "Basic frontpage",
			Data: templates.Args{
				"frontpage": ui.Frontpage{
					Projects: []*projectconfig.Project{
						{
							ID:        "fakeproject",
							HasConfig: true,
							LogoURL:   "https://example.com/logo.png",
						},
					},
				},
			},
		},
	}
}

func relatedBuildsTableTestData() []TestBundle {
	bundles := []TestBundle{}
	for _, tc := range []string{"MacTests", "scheduled"} {
		build, err := GetTestBuild("../buildsource/buildbucket", tc)
		if err != nil {
			panic(fmt.Errorf("Encountered error while fetching %s.\n%s", tc, err))
		}
		bundles = append(bundles, TestBundle{
			Description: fmt.Sprintf("Test related builds table: %s", tc),
			Data: templates.Args{
				"RelatedBuildsTable": &ui.RelatedBuildsTable{
					Build: ui.Build{
						Build: build,
						Now:   nowTS,
					},
					RelatedBuilds: []*ui.Build{{
						Build: build,
						Now:   nowTS,
					}},
				},
			},
		})
	}
	return bundles
}

// GetTestBuild returns a debug build from testdata.
func GetTestBuild(relDir, name string) (*buildbucketpb.Build, error) {
	fname := fmt.Sprintf("%s.build.jsonpb", name)
	path := filepath.Join(relDir, "testdata", fname)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result := &buildbucketpb.Build{}
	return result, jsonpb.Unmarshal(f, result)
}

func TestCreateInterpolator(t *testing.T) {
	ftt.Run("Test createInterpolator", t, func(t *ftt.Test) {
		t.Run("Should encode params", func(t *ftt.Test) {
			params := httprouter.Params{httprouter.Param{Key: "component2", Value: ":? +"}}
			interpolator := createInterpolator("/component1/:component2")

			path := interpolator(params)
			assert.Loosely(t, path, should.Equal("/component1/"+url.PathEscape(":? +")))
		})

		t.Run("Should support catching path segments with *", func(t *ftt.Test) {
			params := httprouter.Params{httprouter.Param{Key: "component2", Value: "/:?/ +"}}
			interpolator := createInterpolator("/component1/*component2")

			path := interpolator(params)
			assert.Loosely(t, path, should.Equal("/component1/"+url.PathEscape(":?")+"/"+url.PathEscape(" +")))
		})

		t.Run("Should support encoding / with *_", func(t *ftt.Test) {
			params := httprouter.Params{httprouter.Param{Key: "_component2", Value: "/:?/ +"}}
			interpolator := createInterpolator("/component1/*_component2")

			path := interpolator(params)
			assert.Loosely(t, path, should.Equal("/component1/"+url.PathEscape(":?/ +")))
		})
	})
}

func TestRedirect(t *testing.T) {
	ftt.Run("Test redirect", t, func(t *ftt.Test) {
		client := &http.Client{
			// Don't follow the redirect. We want to test the response directly.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		r := router.New()
		ts := httptest.NewServer(r)

		t.Run("Should not double-encode params", func(t *ftt.Test) {
			r.GET("/foo/:param", router.NewMiddlewareChain(), redirect("/bar/:param", http.StatusFound))
			res, err := client.Get(ts.URL + "/foo/" + url.PathEscape(":? "))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, res.Header.Get("Location"), should.Equal("/bar/"+url.PathEscape(":? ")))
		})
	})
}
