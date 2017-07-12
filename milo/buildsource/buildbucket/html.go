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

package buildbucket

import (
	"errors"
	"net/http"

	"golang.org/x/net/context"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

func parseBuilderQuery(c context.Context, r *http.Request, p httprouter.Params) (
	query builderQuery, err error) {

	query.Bucket = p.ByName("bucket")
	if query.Bucket == "" {
		err = errors.New("No bucket")
		return
	}

	query.Builder = p.ByName("builder")
	if query.Builder == "" {
		err = errors.New("No builder")
		return
	}

	// limit is a name of the query string parameter for specifying
	// maximum number of builds to show.
	query.Limit, err = common.GetLimit(r)
	return
}

// BuilderHandler renders the builder view page.
// Note: The builder html template contains self links to "?limit=123", which could
// potentially override any other request parameters set.
func BuilderHandler(c *router.Context) {
	query, err := parseBuilderQuery(c.Context, c.Request, c.Params)
	if err != nil {
		common.ErrorPage(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := builderImpl(c.Context, query)
	if err != nil {
		common.ErrorPage(c, http.StatusInternalServerError, err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/builder.html", templates.Args{
		"Builder": result,
	})
}
