// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/miloerror"
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
	"github.com/luci/luci-go/server/templates"
)

// Log is for fetching logs from swarming.
type Log struct{}

// Build is for deciphering recipe builds from swarming based off of logs.
type Build struct{}

// GetTemplateName for Log returns the template name for log pages.
func (l Log) GetTemplateName(t settings.Theme) string {
	return "log.html"
}

// Render writes the build log to the given response writer.
func (l Log) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	id := p.ByName("id")
	if id == "" {
		return nil, &miloerror.Error{
			Message: "No id",
			Code:    http.StatusBadRequest,
		}
	}
	logname := p.ByName("logname")
	if logname == "" {
		return nil, &miloerror.Error{
			Message: "No log name",
			Code:    http.StatusBadRequest,
		}
	}
	server := p.ByName("server") // This one may be blank.
	log, err := swarmingBuildLogImpl(c, server, id, logname)
	if err != nil {
		return nil, err
	}

	args := &templates.Args{
		"Log": log,
	}
	return args, nil
}

// GetTemplateName for Build returns the template name for build pages.
func (b Build) GetTemplateName(t settings.Theme) string {
	return "build.html"
}

// Render renders both the build page and the log.
func (b Build) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	// Get the swarming ID
	id := p.ByName("id")
	if id == "" {
		return nil, &miloerror.Error{
			Message: "No id",
			Code:    http.StatusBadRequest,
		}
	}
	server := p.ByName("server") // This one may be blank.

	result, err := swarmingBuildImpl(c, r.URL.String(), server, id)
	if err != nil {
		return nil, err
	}

	// Render into the template
	args := &templates.Args{
		"Build": result,
	}
	return args, nil
}
