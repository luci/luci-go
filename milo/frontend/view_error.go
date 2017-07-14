// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/milo/common"
)

// ErrorHandler renders an error page for the user.
func ErrorHandler(c *router.Context, err error) {
	// TODO(iannucci): tag/extract other information from error, like a link to the
	// 'container'; i.e. a build may link to its builder, a builder to its
	// master/bucket, etc.

	code := common.ErrorTag.In(err)
	if code == common.CodeUnknown {
		errors.Log(c.Context, err)
	}
	status := code.HTTPStatus()
	c.Writer.WriteHeader(status)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", templates.Args{
		"Code":    status,
		"Message": err.Error(),
	})
}
