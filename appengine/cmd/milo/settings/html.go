// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/xsrf"
	"github.com/luci/luci-go/server/templates"
)

// Settings - Containder for html methods for settings.
type Settings struct{}

// GetTemplateName - Implements a Theme, template is constant.
func (s Settings) GetTemplateName(t Theme) string {
	return "settings.html"
}

// Render renders both the build page and the log.
func (s Settings) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	var u *updateReq
	if r.Method == "POST" {
		// Check XSRF token.
		err := xsrf.Check(c, r.FormValue("xsrf_token"))
		if err != nil {
			return nil, err
		}
		u = &updateReq{
			Theme: r.FormValue("theme"),
		}
	}

	result, err := settingsImpl(c, r.URL.String(), u)
	if err != nil {
		return nil, err
	}

	token, err := xsrf.Token(c)
	if err != nil {
		return nil, err
	}

	args := &templates.Args{
		"Settings":  result,
		"XsrfToken": token,
	}
	return args, nil
}
