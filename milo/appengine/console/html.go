// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package console

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/milo/appengine/settings"
	"github.com/luci/luci-go/milo/common/miloerror"
	"github.com/luci/luci-go/server/templates"
)

type Console struct{}

// GetTemplateName returns the template name for console pages.
func (x Console) GetTemplateName(t settings.Theme) string {
	return "console.html"
}

type ConsoleDef struct {
	Name       string
	Repository string
	Branch     string
	// TODO(hinoka): Replace this with resp.BuilderRef when tmpBuilderRef is removed.
	Builders []tmpBuilderRef
}

// This is like a resp.BuilderRef, but the Categories are pipe deliminated strings.
// This is so that it's easier to define inline here, and this will be deleted once
// console definitions are moved to luci-cfg.
type tmpBuilderRef struct {
	Module    string
	Name      string
	Category  string
	ShortName string
}

// TODO(hinoka): Move all this into some sorta luci-config thing
var chromiumConsoleDef = ConsoleDef{
	Name:       "Chromium Main Waterfall",
	Repository: "https://chromium.googlesource.com/chromium/src",
	Branch:     "master",
	Builders: []tmpBuilderRef{
		{"buildbot", "chromium/Android", "clobber", "an"},
		{"buildbot", "chromium/Linux x64", "clobber", "lx"},
		{"buildbot", "chromium/Mac", "clobber", "mc"},
		{"buildbot", "chromium/Win", "clobber|win", "32"},
		{"buildbot", "chromium/Win x64", "clobber|win", "64"},
		{"buildbot", "chromium.linux/Android Arm64 Builder (dbg)", "linux|android|arm", "db"},
		{"buildbot", "chromium.linux/Android Builder", "linux|android|x86", "rl"},
		{"buildbot", "chromium.linux/Android Builder (dbg)", "linux|android|x86", "db"},
		{"buildbot", "chromium.linux/Android Clang Builder (dbg)", "linux|android|clang", "db"},
		{"buildbot", "chromium.linux/Android Tests", "linux|android|tests", "db"},
		{"buildbot", "chromium.linux/Android Tests (dbg)", "linux|android|tests", "db"},
		{"buildbot", "chromium.linux/Blimp Linux (dbg)", "linux", "bl"},
		{"buildbot", "chromium.linux/Cast Android (dbg)", "linux|cast", "an"},
		{"buildbot", "chromium.linux/Cast Linux", "linux|cast", "lx"},
		{"buildbot", "chromium.linux/Linux Builder", "linux|build", "rl"},
		{"buildbot", "chromium.linux/Linux Builder (dbg)", "linux|build", "d6"},
		{"buildbot", "chromium.linux/Linux Builder (dbg)(32)", "linux|build", "d3"},
		{"buildbot", "chromium.linux/Linux Tests", "linux|test", "rl"},
		{"buildbot", "chromium.linux/Linux Tests (dbg)(1)", "linux|test", "d1"},
		{"buildbot", "chromium.linux/Linux Tests (dbg)(1)(32)", "linux|test", "d2"},
	},
}

// Render renders the console page.
func (x Console) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	// Ignore, hardcoded for demo
	name := p.ByName("name")
	if name == "" {
		return nil, &miloerror.Error{
			Message: "No name",
			Code:    http.StatusBadRequest,
		}
	}

	result, err := console(c, &chromiumConsoleDef)
	if err != nil {
		return nil, err
	}

	// Render into the template
	args := &templates.Args{
		"Console": result,
	}
	return args, nil
}
