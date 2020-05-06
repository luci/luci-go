// Copyright 2020 The LUCI Authors.
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
	"html/template"
	"net/http"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"
)

func main() {
	// Install only default LUCI routes (admin routes and some auth stuff).
	//
	// This tiny server is needed for two reasons:
	//   1. To serve /auth/api/v1/server/client_id for the frontend.
	//   2. GAE modules can't have static files only, they need some Go code.
	r := router.New()
	standard.InstallHandlers(r)

	baseMW := standard.Base()
	r.GET("/settings.js", baseMW, handleError(settingsHandler))
	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}

type settings struct {
	ResultDB resultDBSettings
}

type resultDBSettings struct {
	Host string
}

func settingsHandler(c *router.Context) error {
	template, err := template.ParseFiles("settings.template.js")
	if err != nil {
		return err
	}
	c.Writer.Header().Set("content-type", "application/javascript")
	// TODO(weiweilin): read the host name from luci-config.
	err = template.Execute(c.Writer, settings{resultDBSettings{"staging.results.api.cr.dev"}})
	if err != nil {
		return err
	}
	return nil
}

// handleError is a wrapper for a handler so that the handler can return an
// error.
// If the wrapped handler returns an error, log the error and respond 500.
func handleError(handler func(c *router.Context) error) func(c *router.Context) {
	return func(c *router.Context) {
		if err := handler(c); err != nil {
			errors.Log(c.Context, err)
			c.Writer.WriteHeader(500)
			return
		}
	}
}
