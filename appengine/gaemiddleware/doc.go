// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package gaemiddleware provides a standard middleware for Appengine apps.
//
// This middleware configures the request environment to use GAE-based services
// (like datastore via luci/gae package, logging and many more).
//
// The minimal usage example (from GAE standard):
//  import (
//    ...
//
//    "github.com/luci/luci-go/appengine/gaemiddleware"
//    "github.com/luci/luci-go/common/logging"
//    "github.com/luci/luci-go/server/router"
//  )
//
//  func init() {
//    r := router.New()
//    gaemiddleware.InstallHandlers(r)
//
//    r.GET("/", gaemiddleware.BaseProd(), indexPage)
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
