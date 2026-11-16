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

package ui

import (
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/scheduler/appengine/engine"
)

// redirectToLogin redirects anonymous GET requests to the login page and
// returns true, or returns false if the request is already authenticated or is
// not a GET request.
func redirectToLogin(c *router.Context) bool {
	if c.Request.Method != http.MethodGet {
		return false
	}
	ctx := c.Request.Context()
	if auth.CurrentIdentity(ctx) != identity.AnonymousIdentity {
		return false
	}
	loginURL, err := auth.LoginURL(ctx, c.Request.URL.RequestURI())
	if err != nil {
		panic(err)
	}
	http.Redirect(c.Writer, c.Request, loginURL, http.StatusFound)
	return true
}

// jobFromEngine fetches an *engine.Job and returns it, or writes HTTP Not Found
// reply (or login redirect for anonymous GET requests) to the response writer
// and returns nil if it is not found.
//
// Panics on unexpected errors, so panic catcher middleware can write HTTP
// Internal Server Error page.
func jobFromEngine(c *router.Context, projectID, jobName string) *engine.Job {
	job, err := config(c.Request.Context()).Engine.GetVisibleJob(c.Request.Context(), projectID+"/"+jobName)
	switch {
	case err == engine.ErrNoSuchJob:
		if !redirectToLogin(c) {
			uiErrNoJobOrNoPerm.render(c)
		}
		return nil
	case err != nil:
		panic(err)
	default:
		return job
	}
}
