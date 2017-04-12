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
package gaemiddleware
