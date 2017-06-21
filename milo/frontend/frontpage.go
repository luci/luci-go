// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/common/sync/parallel"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/job_source/buildbot"
	"github.com/luci/luci-go/milo/job_source/buildbucket"
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
								*resp.NewLink("Example builder", "/master1/buildera"),
								*resp.NewLink("Example builder 2", "/master1/builderb"),
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
