// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	"fmt"
	"log"
	"net/http"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/miloerror"
	"github.com/luci/luci-go/appengine/cmd/milo/swarming"
)

// Where it all begins!!!
func init() {
	// Register endpoint services.
	ss := &swarming.Service{}
	api, err := endpoints.RegisterService(ss, "swarming", "v1", "Milo Swarming API", true)
	if err != nil {
		log.Printf("Unable to register endpoint services: %s", err)
	} else {
		register := func(orig, name, method, path, desc string) {
			m := api.MethodByName(orig)
			i := m.Info()
			i.Name, i.HTTPMethod, i.Path, i.Desc = name, method, path, desc
			return
		}
		register("Build", "swarming.build", "GET", "swarming", "Swarming Build view.")
	}

	// Register plain ol' http services.
	r := httprouter.New()
	r.GET("/", gaemiddleware.BaseProd(wrap(root)))
	// TODO(hinoka): Handle different swarming servers, eg dev vs prod.
	r.GET(
		"/swarming/:id/steps/:step/logs/:log",
		gaemiddleware.BaseProd(wrap(swarming.WriteBuildLog)))
	r.GET("/swarming/:id", gaemiddleware.BaseProd(wrap(swarming.Render)))
	http.Handle("/", r)

	endpoints.HandleHTTP()
}

// Dummy page to serve "/"
func root(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	fmt.Fprintf(w, `This is the root page`)
	return nil
}

type handler func(context.Context, http.ResponseWriter, *http.Request, httprouter.Params) error

// Wrapper around handler functions so that they can return errors and be
// rendered as 400's
func wrap(h handler) middleware.Handler {
	return func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := h(c, w, r, p); err != nil {
			if merr, ok := err.(*miloerror.Error); ok {
				http.Error(w, merr.Message, merr.Code)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}
}
