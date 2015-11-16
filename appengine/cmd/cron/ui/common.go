// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ui implements request handlers that serve user facing HTML pages.
package ui

import (
	"strings"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/gae/service/info"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/appengine/cmd/cron/engine"
)

// Config is global configuration of UI handlers.
type Config struct {
	Engine        engine.Engine
	TemplatesPath string // path to templates directory deployed to GAE
}

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(r *httprouter.Router, base middleware.Base, cfg Config) {
	tmpl := prepareTemplates(cfg.TemplatesPath)

	wrap := func(h middleware.Handler) httprouter.Handle {
		h = auth.Authenticate(h)
		h = templates.WithTemplates(h, tmpl)
		h = middleware.WithContextValue(h, configContextKey(0), &cfg)
		if !appengine.IsDevAppServer() {
			h = middleware.WithPanicCatcher(h)
		}
		return base(h)
	}

	r.GET("/", wrap(indexPage))
}

type configContextKey int

// config returns Config passed to InstallHandlers.
func config(c context.Context) *Config {
	cfg, _ := c.Value(configContextKey(0)).(*Config)
	if cfg == nil {
		panic("impossible, configContextKey is not set")
	}
	return cfg
}

// prepareTemplates configures templates.Bundle used by all UI handlers.
//
// In particular it includes a set of default arguments passed to all templates.
func prepareTemplates(templatesPath string) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templatesPath),
		DebugMode:       appengine.IsDevAppServer(),
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context) (templates.Args, error) {
			loginURL, err := auth.LoginURL(c, "/")
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(c, "/")
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"AppVersion":  strings.Split(info.Get(c).VersionID(), ".")[0],
				"IsAnonymous": auth.CurrentIdentity(c) == "anonymous:anonymous",
				"User":        auth.CurrentUser(c),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
			}, nil
		},
	}
}
