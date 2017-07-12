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

package console

import (
	"net/http"

	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// ConsoleHandler renders the console page.
func ConsoleHandler(c *router.Context) {
	project := c.Params.ByName("project")
	if project == "" {
		common.ErrorPage(c, http.StatusBadRequest, "Missing Project")
		return
	}
	name := c.Params.ByName("name")

	result, err := console(c.Context, project, name)
	if err != nil {
		common.ErrorPage(c, http.StatusInternalServerError, err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/console.html", templates.Args{
		"Console": result,
	})
}
