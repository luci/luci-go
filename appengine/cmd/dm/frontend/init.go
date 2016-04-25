// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/cmd/dm/deps"
	gaeauthServer "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	adminSettings "github.com/luci/luci-go/server/settings/admin"
)

func base(h middleware.Handler) httprouter.Handle {
	newH := func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		cfg, err := gaeconfig.New(c)
		switch err {
		case nil:
			c = config.Set(c, cfg)
		case gaeconfig.ErrNotConfigured:
			logging.Warningf(c, "luci-config service url not configured. Configure this at /admin/settings/gaeconfig.")
		default:
			panic(err)
		}
		h(c, rw, r, p)
	}
	return gaemiddleware.BaseProd(newH)
}

func init() {
	router := httprouter.New()
	tmb := tumble.Service{}

	svr := prpc.Server{}
	deps.RegisterDepsServer(&svr)
	discovery.Enable(&svr)

	svr.InstallHandlers(router, base)
	tmb.InstallHandlers(router)
	gaeauthServer.InstallHandlers(router, base)
	adminSettings.InstallHandlers(router, base, &gaeauthServer.UsersAPIAuthMethod{})

	http.Handle("/", router)
}
