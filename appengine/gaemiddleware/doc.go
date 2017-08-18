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

// Package gaemiddleware provides a standard middleware for Appengine apps. The
// gaemiddleware package itself provides a generic Environment class and
// common implementations of methods. An Environment matching your AppEngine
// environment configuration (e.g., standard, flex) should be chosen from a
// sub-package.
//
// This middleware configures the request environment to use GAE-based services
// (like datastore via luci/gae package, logging and many more).
//
// The minimal usage example (from GAE standard):
//  import (
//    ...
//
//    "go.chromium.org/luci/appengine/gaemiddleware/standard"
//    "go.chromium.org/luci/common/logging"
//    "go.chromium.org/luci/server/router"
//  )
//
//  func init() {
//    r := router.New()
//    standard.InstallHandlers(r)
//
//    r.GET("/", standard.Base(), indexPage)
//
//    http.DefaultServeMux.Handle("/", r)
//  }
//
//  func indexPage(c *router.Context) {
//    logging.Infof(c.Context, "Handling the page")
//    ...
//  }
//
// It registers various routes required for LUCI framework functionality in the
// router, and sets up an application route (indexPage) configured to use GAE
// services.
//
// Handlers setup
//
// Some default registered routes are intended for use only by administrators or
// internally by GAE itself (crons, task queues). While LUCI framework does
// authorization checks itself, it is still recommended that you protect such
// routes on app.yaml level with "login: admin" check, to make GAE reject
// unauthorized access earlier.
//
// In contrast, the rest of the routes (end-user facing HTML pages, API
// handlers) usually use LUCI's authentication framework (to support OAuth2
// tokens, among other things), and for that reason they must not use
// "login: required" or "login: admin", since in that case GAE will enable its
// own cookie-based authentication mechanism (that doesn't work with OAuth2).
//
// Thus the recommended handlers list is:
//  handlers:
//  - url: /(internal|admin)/.*
//    script: _go_app
//    secure: always
//    login: admin
//
//  - url: /.*
//    script: _go_app
//    secure: always
//
// See https://cloud.google.com/appengine/docs/standard/go/config/appref for
// more info about app.yaml.
//
// Cron setup
//
// Some of the default LUCI services installed in BaseProd require cron jobs
// for their operation.
//
// InstallHandlers call registers the cron handlers in the HTTP router, but GAE
// still has to be instructed to actually call them.
//
// This can done by adding following jobs to cron.yaml file of your project:
//
//   - description: "tsmon housekeeping task"
//     url: /internal/cron/ts_mon/housekeeping
//     schedule: every 1 minutes
//
//   - description: "LUCI Config datastore cache periodic refresh"
//     url: /admin/config/cache/manager
//     schedule: every 10 mins
//
// See https://cloud.google.com/appengine/docs/standard/go/config/cronref for
// more information about cron.yaml.
package gaemiddleware
