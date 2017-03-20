// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/common/sync/parallel"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/buildbot"
	"github.com/luci/luci-go/milo/appengine/buildbucket"
	"github.com/luci/luci-go/milo/appengine/common"
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
	if err != nil {
		common.ErrorPage(c, http.StatusInternalServerError, err.Error())
		return
	}

	fp.CIServices = append(fp.CIServices, *mBuildbucket)
	fp.CIServices = append(fp.CIServices, *mBuildbot)
	templates.MustRender(c.Context, c.Writer, "pages/frontpage.html", templates.Args{
		"frontpage": fp,
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
								{
									Label: "Example builder",
									URL:   "/master1/buildera",
								},
								{
									Label: "Example builder 2",
									URL:   "/master1/builderb",
								},
							},
						},
					},
				},
			},
		},
	}
	return []common.TestBundle{
		{
			Description: "Basic frontpage",
			Data:        *data,
		},
	}
}
