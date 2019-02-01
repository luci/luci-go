// Copyright 2019 The LUCI Authors.
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
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/adminsrv"
	"html/template"
)

var (
	form = template.Must(template.New("form").Parse(`
	<html>
    <head>
    <title>Scoped Service Account Lookup</title>
    </head>
    <body>
        <form action="/ui" method="post">
            Account ID or Email:<input type="text" name="accountid">
            <input type="submit" value="Lookup">
        </form>
    </body>
  </html>`))

	result = template.Must(template.New("result").Parse(`
  <html>
    <head>
      <title>Scoped Service Account Lookup</title>
    </head>
    <body>
      Project Name: {{.Project}}, Service Name: {{.Service}}
    </body>
  </html>`))
)

// UI is the top-level data structure for handling tokenserver UI elements.
type UI struct {
	server *adminsrv.AdminServer
}

// ShowFormHandler shows the scoped service accounts lookup form.
func (s *UI) ShowFormHandler(c *router.Context) {
	form.Execute(c.Writer, nil)
}

// ProcessFormHandler processes the scoped service accounts lookup form and displays the results.
func (s *UI) ProcessFormHandler(c *router.Context) {
	err := c.Request.ParseForm()
	if err != nil {
		panic(err)
	}

	req := &admin.LookupProjectScopedServiceAccountRequest{
		AccountEmail: c.Request.Form.Get("accountid"),
	}

	response, err := s.server.LookupProjectScopedServiceAccount(c.Context, req)
	if err != nil {
		panic(err)
	}

	params := struct {
		Project string
		Service string
	}{
		Project: response.Project,
		Service: response.Service,
	}

	result.Execute(c.Writer, params)
}

// NewUI creates a new UI structure.
func NewUI(server *adminsrv.AdminServer) *UI {
	return &UI{server}
}
