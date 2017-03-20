// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// ErrorPage writes an error page into c.Writer with an http code "code"
// and custom message.
func ErrorPage(c *router.Context, code int, message string) {
	c.Writer.WriteHeader(code)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", templates.Args{
		"Code":    code,
		"Message": message,
	})
}
