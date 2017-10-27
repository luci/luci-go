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
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
)

func frontpageHandler(c *router.Context) {
	fp := resp.FrontPage{}
	var mBuildbot, mBuildbucket *resp.CIService

	err := parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() (err error) {
			mBuildbot, err = buildbot.GetAllBuilders(c.Context)
			return err
		}
		ch <- func() (err error) {
			mBuildbucket, err = buildbucket.GetAllBuilders(c.Context)
			return err
		}
	})

	fp.CIServices = append(fp.CIServices, *mBuildbucket)
	fp.CIServices = append(fp.CIServices, *mBuildbot)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	templates.MustRender(c.Context, c.Writer, "pages/frontpage.html", templates.Args{
		"frontpage": fp,
		"error":     errMsg,
	})
}

func frontpageTestData() []common.TestBundle {
	data := &templates.Args{
		"frontpage": resp.FrontPage{
			CIServices: []resp.CIService{
				{
					Name: "Module 1",
					BuilderGroups: []resp.BuilderGroup{
						{
							Name: "Example master A",
							Builders: []resp.Link{
								*resp.NewLink("Example builder", "/master1/buildera", "Example label"),
								*resp.NewLink("Example builder 2", "/master1/builderb", "Example label 2"),
							},
						},
					},
				},
			},
		},
		"error": "couldn't find ice cream",
	}
	return []common.TestBundle{
		{
			Description: "Basic frontpage",
			Data:        *data,
		},
	}
}
