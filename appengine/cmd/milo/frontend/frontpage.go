// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/settings"
)

type frontpage struct{}

func (f frontpage) GetTemplateName(t settings.Theme) string {
	return "frontpage.html"
}

func (f frontpage) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	return &templates.Args{
		"Contents": "Herro thar, this is Milo!",
		"Image":    "https://storage.googleapis.com/luci-milo/milo.jpg",
	}, nil
}

type testableFrontpage struct{ frontpage }

func (l testableFrontpage) TestData() []settings.TestBundle {
	data, _ := l.Render(nil, nil, nil)
	return []settings.TestBundle{
		{
			Description: "Basic frontpage",
			Data:        *data,
		},
	}
}
