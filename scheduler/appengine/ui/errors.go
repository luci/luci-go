// Copyright 2018 The LUCI Authors.
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

package ui

import (
	"net/http"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// List of all possible error messages presented to the user.
var (
	uiErrNoJobOrNoPerm = presentableError{
		Status:         http.StatusNotFound,
		Message:        "No such job or no READER permission to view it.",
		ReloginMayHelp: true,
	}

	uiErrBadInvocationID = presentableError{
		Status:         http.StatusBadRequest,
		Message:        "Malformed invocation ID.",
		ReloginMayHelp: false,
	}

	uiErrNoInvocation = presentableError{
		Status:         http.StatusNotFound,
		Message:        "The requested invocation is not found.",
		ReloginMayHelp: false,
	}

	uiErrActionForbidden = presentableError{
		Status:         http.StatusForbidden,
		Message:        "No permission to execute the attempted action.",
		ReloginMayHelp: true,
	}
)

// presentableError defines an error message we render as a pretty HTML page.
//
// All possible kinds of errors are instantiated above.
type presentableError struct {
	Status         int    // HTTP status code
	Message        string // the main message on the page
	ReloginMayHelp bool   // if true, display a suggestion to login/relogin
}

// render renders the HTML into the writer and sets the response code.
func (e *presentableError) render(c *router.Context) {
	breadcrumps := []string{}
	for _, k := range []string{"ProjectID", "JobName", "InvID"} {
		p := c.Params.ByName(k)
		if p == "" {
			break
		}
		breadcrumps = append(breadcrumps, p)
	}
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(e.Status)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", map[string]interface{}{
		"Breadcrumps":    breadcrumps,
		"LastCrumbIdx":   len(breadcrumps) - 1,
		"Message":        e.Message,
		"ReloginMayHelp": e.ReloginMayHelp,
	})
}
