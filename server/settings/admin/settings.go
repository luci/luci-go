// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package admin

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/server/templates"
)

type fieldWithValue struct {
	settings.UIField
	Value string
}

type validationError struct {
	FieldTitle string
	Value      string
	Error      string
}

type pageCallback func(id string, p settings.UIPage) error

func withPage(c context.Context, rw http.ResponseWriter, p httprouter.Params, cb pageCallback) {
	id := p.ByName("SettingsKey")
	page := settings.GetUIPages()[id]
	if page == nil {
		rw.WriteHeader(http.StatusNotFound)
		templates.MustRender(c, rw, "pages/error.html", templates.Args{
			"Error": "No such settings",
		})
		return
	}
	if err := cb(id, page); err != nil {
		replyError(c, rw, err)
	}
}

func settingsPageGET(ctx *router.Context) {
	c, rw, p := ctx.Context, ctx.Writer, ctx.Params

	withPage(c, rw, p, func(id string, page settings.UIPage) error {
		title, err := page.Title(c)
		if err != nil {
			return err
		}
		overview, err := page.Overview(c)
		if err != nil {
			return err
		}
		fields, err := page.Fields(c)
		if err != nil {
			return err
		}
		values, err := page.ReadSettings(c)
		if err != nil {
			return err
		}

		withValues := make([]fieldWithValue, len(fields))
		for i, f := range fields {
			withValues[i] = fieldWithValue{
				UIField: f,
				Value:   values[f.ID],
			}
		}

		templates.MustRender(c, rw, "pages/settings.html", templates.Args{
			"ID":             id,
			"Title":          title,
			"Overview":       overview,
			"Fields":         withValues,
			"XsrfTokenField": xsrf.TokenField(c),
		})
		return nil
	})
}

func settingsPagePOST(ctx *router.Context) {
	c, rw, r, p := ctx.Context, ctx.Writer, ctx.Request, ctx.Params

	withPage(c, rw, p, func(id string, page settings.UIPage) error {
		title, err := page.Title(c)
		if err != nil {
			return err
		}
		fields, err := page.Fields(c)
		if err != nil {
			return err
		}

		// Extract values from the page and validate them.
		values := make(map[string]string, len(fields))
		validationErrors := []validationError{}
		for _, f := range fields {
			if !f.Type.IsEditable() {
				continue
			}
			val := r.PostFormValue(f.ID)
			values[f.ID] = val
			if f.Validator != nil {
				if err := f.Validator(val); err != nil {
					validationErrors = append(validationErrors, validationError{
						FieldTitle: f.Title,
						Value:      val,
						Error:      err.Error(),
					})
				}
			}
		}
		if len(validationErrors) != 0 {
			rw.WriteHeader(http.StatusBadRequest)
			templates.MustRender(c, rw, "pages/validation_error.html", templates.Args{
				"ID":     id,
				"Title":  title,
				"Errors": validationErrors,
			})
			return nil
		}

		// Store.
		err = page.WriteSettings(c, values, auth.CurrentUser(c).Email, "modified via web UI")
		if err != nil {
			return err
		}
		templates.MustRender(c, rw, "pages/done.html", templates.Args{
			"ID":    id,
			"Title": title,
		})
		return nil
	})
}
