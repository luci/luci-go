// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/miloerror"
)

var (
	tmpl = template.Must(template.New("build.html").ParseFiles("templates/buildbot/build.html"))
)

// WriteBuildLog writes the build log to the given response writer.
func WriteBuildLog(
	c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	id := p.ByName("id")
	if id == "" {
		return &miloerror.Error{
			Message: "No id",
			Code:    http.StatusBadRequest,
		}
	}
	log := p.ByName("log")
	if log == "" {
		return &miloerror.Error{
			Message: "No log",
			Code:    http.StatusBadRequest,
		}
	}
	step := p.ByName("step")
	if step == "" {
		return &miloerror.Error{
			Message: "No step",
			Code:    http.StatusBadRequest,
		}
	}
	server := p.ByName("server") // This one may be blank.
	b, err := swarmingBuildLogImpl(c, server, id, log, step)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "<pre>%s</pre>", b.log)
	return nil
}

// Render renders both the build page and the log.
func Render(
	c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	// Get the swarming ID
	id := p.ByName("id")
	if id == "" {
		return &miloerror.Error{
			Message: "No id",
			Code:    http.StatusBadRequest,
		}
	}
	server := p.ByName("server") // This one may be blank.

	result, err := swarmingBuildImpl(c, r.URL.String(), server, id)
	if err != nil {
		return err
	}

	// Render into the template
	err = tmpl.Execute(w, *result)
	if err != nil {
		return err
	}
	return nil
}
