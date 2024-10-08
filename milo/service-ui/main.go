// Copyright 2024 The LUCI Authors.
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

// Package main contains some experimental setup for push-on-green. See
// b/367097786.
package main

import (
	"text/template"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"
)

var uiVersionJsTemplateStr = `
	self.UI_VERSION = '{{.Version}}';
`

var uiVersionJsTemplate = template.Must(template.New("ui_version.js").Parse(uiVersionJsTemplateStr))

// main implements the entrypoint for the UI service.
//
// The UI service doesn't do much. It only
//   - hosts the static files, and
//   - provides the UI version via a JS file.
//
// Unless absolutely necessary, all other functionalities should be delegated to
// other services so rolling the UI service has minimal risks.
//
// Note that the UI version can only be served from the UI service because the
// other services do not know the version of the UI service.
func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		srv.Routes.GET("/ui_version.js", nil, func(c *router.Context) {
			header := c.Writer.Header()
			header.Set("content-type", "text/javascript")

			// We don't need to cache the UI version file because it is fetched and
			// re-served by the service worker.
			header.Set("cache-control", "no-cache")
			err := uiVersionJsTemplate.Execute(c.Writer, map[string]any{
				"Version": srv.Options.ImageVersion(),
			})
			if err != nil {
				logging.Errorf(c.Request.Context(), "Failed to execute ui_version.js template: %s", err)
				c.Writer.WriteHeader(500)
				if _, err := c.Writer.Write([]byte(err.Error())); err != nil {
					logging.Warningf(c.Request.Context(), "failed to write response body: %s", err)
					return
				}
			}
		})

		return nil
	})
}
