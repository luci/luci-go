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
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"

	. "github.com/smartystreets/goconvey/convey"
)

var now = time.Date(2019, time.February, 3, 4, 5, 6, 7, time.UTC)
var nowTS, _ = ptypes.TimestampProto(now)

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
		{buildbucketBuildTestData, "buildbucket.build", "build.html"},
		{consoleTestData, "console", "console.html"},
		{Frontpage, "frontpage", "frontpage.html"},
		{Search, "search", "search.html"},
		{builderPageData, "builder", "builder.html"},
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
		c, _ = testclock.UseTime(c, now)
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
			Data: templates.Args{"BuildPage": &ui.BuildPage{
				Build: ui.Build{
					Build: build,
					Now:   nowTS,
				},
				BuildbucketHost: "example.com",
			}},
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
					Status: model.Success,
				},
			},
			nil,
		},
		Builder: &model.BuilderSummary{
			BuilderID:          "buildbucket/luci.project-foo.try/builder-bar",
			ProjectID:          "project-foo",
			LastFinishedStatus: model.InfraFailure,
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
						Oncalls: []ui.Oncall{
							{
								Name: "Sheriff",
								Emails: []string{
									"test@example.com",
									"watcher@example.com",
								},
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
												LastFinishedStatus: model.Success,
											},
											{
												LastFinishedStatus: model.Success,
											},
											{
												LastFinishedStatus: model.Failure,
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
												LastFinishedStatus: model.Success,
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
					Projects: []common.Project{
						{
							ID:      "fakeproject",
							LogoURL: "https://example.com/logo.png",
						},
					},
				},
			},
		},
	}
}

func Search() []TestBundle {
	data := templates.Args{
		"search": ui.Search{
			CIServices: []ui.CIService{
				{
					Name: "Module 1",
					BuilderGroups: []ui.BuilderGroup{
						{
							Name: "Example master A",
							Builders: []ui.Link{
								*ui.NewLink("Example builder", "/master1/buildera", "Example label"),
								*ui.NewLink("Example builder 2", "/master1/builderb", "Example label 2"),
							},
						},
					},
				},
			},
		},
		"error": "couldn't find ice cream",
	}
	return []TestBundle{
		{
			Description: "Basic search page",
			Data:        data,
		},
	}
}

func builderPageData() []TestBundle {

	build := func(b *buildbucketpb.Build) *ui.Build {
		b.Builder = &buildbucketpb.BuilderID{
			Project: "chromium",
			Bucket:  "try",
			Builder: "linux-rel",
		}
		return &ui.Build{
			Build: b,
			Now:   nowTS,
		}
	}

	return []TestBundle{
		{
			Description: "Builder page",
			Data: templates.Args{
				"Request": &http.Request{
					URL: &url.URL{Path: "/p/chromium/builders/try/linux-rel"},
				},
				"BuilderPage": &ui.BuilderPage{
					BuilderName: "linux-rel",
					ScheduledBuilds: []*ui.Build{
						build(&buildbucketpb.Build{
							Id:         1,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
						}),
						build(&buildbucketpb.Build{
							Id:         2,
							Number:     1,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
						}),
					},
					StartedBuilds: []*ui.Build{
						build(&buildbucketpb.Build{
							Id:         3,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
							StartTime:  &timestamp.Timestamp{Seconds: 1544748010},
						}),
						build(&buildbucketpb.Build{
							Id:         4,
							Number:     1,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
							StartTime:  &timestamp.Timestamp{Seconds: 1544748010},
						}),
					},
					EndedBuilds: []*ui.Build{
						build(&buildbucketpb.Build{
							Id:         5,
							Status:     buildbucketpb.Status_SUCCESS,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
							EndTime:    &timestamp.Timestamp{Seconds: 1544748020},
							Input: &buildbucketpb.Build_Input{
								GerritChanges: []*buildbucketpb.GerritChange{
									{
										Host:     "chromium.googlesource.com",
										Project:  "chromium/src",
										Change:   123,
										Patchset: 1,
									},
									{
										Host:     "chromium.googlesource.com",
										Project:  "chromium/src-2",
										Change:   200,
										Patchset: 1,
									},
								},
								GitilesCommit: &buildbucketpb.GitilesCommit{
									Host:    "chromium.googlesource.com",
									Project: "chromium/src",
									Id:      "e57f4e87022d765b45e741e478a8351d9789bc37",
								},
							},
						}),
						build(&buildbucketpb.Build{
							Id:         6,
							Status:     buildbucketpb.Status_FAILURE,
							Number:     1,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
							StartTime:  &timestamp.Timestamp{Seconds: 1544748010},
							EndTime:    &timestamp.Timestamp{Seconds: 1544748020},
							Input: &buildbucketpb.Build_Input{
								GerritChanges: []*buildbucketpb.GerritChange{
									{
										Host:     "chromium.googlesource.com",
										Project:  "chromium/src",
										Change:   123,
										Patchset: 1,
									},
								},
							},
							Output: &buildbucketpb.Build_Output{
								GitilesCommit: &buildbucketpb.GitilesCommit{
									Host:     "chromium.googlesource.com",
									Project:  "chromium/src",
									Ref:      "refs/heads/master",
									Id:       "e57f4e87022d765b45e741e478a8351d9789bc37",
									Position: 32,
								},
							},
						}),
						build(&buildbucketpb.Build{
							Id:         6,
							Status:     buildbucketpb.Status_FAILURE,
							Number:     1,
							CreateTime: &timestamp.Timestamp{Seconds: 1544748000},
							StartTime:  &timestamp.Timestamp{Seconds: 1544748010},
							EndTime:    &timestamp.Timestamp{Seconds: 1544748020},
							Output: &buildbucketpb.Build_Output{
								GitilesCommit: &buildbucketpb.GitilesCommit{
									Host:     "chromium.googlesource.com",
									Project:  "chromium/src",
									Ref:      "refs/heads/master",
									Id:       "e57f4e87022d765b45e741e478a8351d9789bc37",
									Position: 32,
								},
							},
						}),
					},
				},
			},
		},
	}
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
