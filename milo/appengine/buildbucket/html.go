// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/milo/appengine/settings"
	"github.com/luci/luci-go/milo/common/miloerror"
	"github.com/luci/luci-go/server/templates"
)

// TODO(nodir): move this value to luci-config.
const defaultServer = "cr-buildbucket.appspot.com"

// Builder displays builder view by fetching builds from buildbucket.
type Builder struct{}

// GetTemplateName for Builder returns the template name for builder pages.
func (b Builder) GetTemplateName(t settings.Theme) string {
	return "builder.html"
}

// Render renders builder view page.
func (b Builder) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	// Parse URL parameters.
	server := r.FormValue("server")
	if server == "" {
		server = defaultServer
	}

	bucket := p.ByName("bucket")
	if bucket == "" {
		return nil, &miloerror.Error{
			Message: "No bucket",
			Code:    http.StatusBadRequest,
		}
	}

	builder := p.ByName("builder")
	if builder == "" {
		return nil, &miloerror.Error{
			Message: "No builder",
			Code:    http.StatusBadRequest,
		}
	}

	// limit is a name of the query string parameter for specifying
	// maximum number of builds to show.
	limit, err := settings.GetLimit(r)
	if err != nil {
		return nil, err
	}

	result, err := builderImpl(c, server, bucket, builder, limit)
	if err != nil {
		return nil, err
	}

	// Render into the template
	args := &templates.Args{
		"Builder": result,
	}
	return args, nil
}
