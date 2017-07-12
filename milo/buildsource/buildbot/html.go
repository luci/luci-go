// Copyright 2015 The LUCI Authors.
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

package buildbot

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// BuildHandler Renders the buildbot build page.
func BuildHandler(c *router.Context) {
	master := c.Params.ByName("master")
	if master == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No master specified")
		return
	}
	builder := c.Params.ByName("builder")
	if builder == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No builder specified")
		return
	}
	buildNum := c.Params.ByName("build")
	if buildNum == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No build number")
		return
	}
	num, err := strconv.Atoi(buildNum)
	if err != nil {
		common.ErrorPage(c, http.StatusBadRequest,
			fmt.Sprintf("%s does not look like a number", buildNum))
		return
	}

	result, err := Build(c.Context, master, builder, num)
	if err != nil {
		var code int
		switch err {
		case errBuildNotFound:
			code = http.StatusNotFound
		case errNotAuth:
			code = http.StatusUnauthorized
		default:
			code = http.StatusInternalServerError
		}
		common.ErrorPage(c, code, err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"Build": result,
	})
}

// BuilderHandler renders the buildbot builder page.
// Note: The builder html template contains self links to "?limit=123", which could
// potentially override any other request parameters set.
func BuilderHandler(c *router.Context) {
	master := c.Params.ByName("master")
	if master == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No master specified")
		return
	}
	builder := c.Params.ByName("builder")
	if builder == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No builder specified")
		return
	}
	limit, err := common.GetLimit(c.Request)
	if err != nil {
		common.ErrorPage(c, http.StatusBadRequest, err.Error())
		return
	}
	if limit < 0 {
		limit = 25
	}
	cursor := c.Request.FormValue("cursor")

	result, err := builderImpl(c.Context, master, builder, limit, cursor)
	_, notFound := err.(errBuilderNotFound)
	switch {
	case err == nil:
		templates.MustRender(c.Context, c.Writer, "pages/builder.html", templates.Args{
			"Builder": result,
		})
	case err == errNotAuth:
		common.ErrorPage(c, http.StatusUnauthorized, err.Error())
	case notFound:
		common.ErrorPage(c, http.StatusNotFound, err.Error())
	default: // err != nil
		common.ErrorPage(c, http.StatusInternalServerError, err.Error())
	}
	return
}
