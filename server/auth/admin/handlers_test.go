// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/secrets/testsecrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAccess(t *testing.T) {
	Convey("Works for admin", t, func() {
		c := makeContext()
		router := httprouter.New()
		InstallHandlers(router, middleware.TestingBase(c), authtest.FakeAuth{
			&auth.User{
				Identity:  "user:admin@example.com",
				Superuser: true,
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/admin/settings", nil)
		router.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 200)
	})

	Convey("Doesn't works for non-admin", t, func() {
		c := makeContext()
		router := httprouter.New()
		InstallHandlers(router, middleware.TestingBase(c), authtest.FakeAuth{
			&auth.User{
				Identity:  "user:non-admin@example.com",
				Superuser: false,
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/admin/settings", nil)
		router.ServeHTTP(w, req)
		So(w.Code, ShouldEqual, 403)
	})
}

func makeContext() context.Context {
	return testsecrets.Use(context.Background())
}
