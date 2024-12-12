// Copyright 2022 The LUCI Authors.
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

package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/analysis/internal/ui"
	analysisserver "go.chromium.org/luci/analysis/server"
)

// Entrypoint for the default service.
func main() {
	analysisserver.Main(func(srv *server.Server) error {
		uiBaseURL, ok := srv.Context.Value(&ui.UIBaseURLKey).(string)

		if !ok {
			return errors.New("UI base URL is not configured in context")
		}

		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			url := fmt.Sprintf("https://%s", uiBaseURL)
			// Remove the `tests/` suffix to redirect to LUCI UI base.
			// This is because LUCI Analysis has a project selection page
			// while the migrated version does not as the project
			// selection happens at the base URL of LUCI UI, and hence users should
			// be redirected to the base URL to select a project.
			url = strings.TrimSuffix(url, "tests/")
			http.Redirect(ctx.Writer, ctx.Request, url, http.StatusFound)
		})

		srv.Routes.NotFound(nil, func(ctx *router.Context) {
			url := fmt.Sprintf("https://%s%s", uiBaseURL, ctx.Request.URL.Path)
			http.Redirect(ctx.Writer, ctx.Request, url, http.StatusFound)
		})

		return nil
	})
}
