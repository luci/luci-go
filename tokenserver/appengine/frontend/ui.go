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
	"html/template"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/adminsrv"
)

var (
	form = template.Must(template.New("form").Parse(`
	<html>
    <head>
    <title>Scoped Service Account Lookup</title>
    </head>
    <body>
        <form action="/ui/scoped/lookup" method="post">
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

	createShow = template.Must(template.New("createShow").Parse(`
	<html>
    <head>
    <title>Scoped Service Account Lookup</title>
    </head>
    <body>
        <form action="/ui/scoped/create" method="post">
            Service:<input type="text" name="service"><br>
            Project:<input type="text" name="project"><br>
            <input type="submit" value="Create">
        </form>
    </body>
  </html>`))

	createResult = template.Must(template.New("createResult").Parse(`
  <html>
    <head>
      <title>Scoped Service Account Creation</title>
    </head>
    <body>
      Project Name: {{.Project}}, Service Name: {{.Service}}, Account Email: {{.Email}}
		</body>
	</html>`))
)

// UI is the top-level data structure for handling tokenserver UI elements.
type UI struct {
	server *adminsrv.AdminServer
}

// ShowCreateScopedAccountFormHandler shows the scoped service accounts creation form.
func (s *UI) ShowCreateScopedAccountFormHandler(c *router.Context) {
	createShow.Execute(c.Writer, nil)
}

// ProcessCreateScopedAccountFormHandler processed the scoped service accounts creation form.
func (s *UI) ProcessCreateScopedAccountFormHandler(c *router.Context) {
	err := c.Request.ParseForm()
	if err != nil {
		panic(err)
	}

	req := &admin.CreateProjectScopedServiceAccountRequest{
		Service: c.Request.Form.Get("service"),
		Project: c.Request.Form.Get("project"),
	}

	response, err := s.server.CreateProjectScopedServiceAccount(c.Context, req)
	if err != nil {
		panic(err)
	}

	params := struct {
		Project string
		Service string
		Email   string
	}{
		Service: req.Service,
		Project: req.Project,
		Email:   response.AccountEmail,
	}

	createResult.Execute(c.Writer, params)
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
