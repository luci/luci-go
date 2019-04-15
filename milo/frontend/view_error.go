// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
	"google.golang.org/grpc/codes"
)

// ErrorHandler renders an error page for the user.
func ErrorHandler(c *router.Context, err error) {
	code := grpcutil.Code(err)
	switch code {
	case codes.Unauthenticated:
		loginURL, err := auth.LoginURL(c.Context, c.Request.URL.RequestURI())
		if err == nil {
			http.Redirect(c.Writer, c.Request, loginURL, http.StatusFound)
			return
		}
		errors.Log(
			c.Context, errors.Annotate(err, "Failed to retrieve login URL").Err())
	case codes.OK:
		// All good.
	default:
		errors.Log(c.Context, err)
	}

	status := grpcutil.CodeStatus(code)
	c.Writer.WriteHeader(status)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", templates.Args{
		"Code":    status,
		"Message": err.Error(),
	})
}
