// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/dm/appengine/deps"
	"github.com/luci/luci-go/dm/appengine/distributor"
	"github.com/luci/luci-go/dm/appengine/distributor/jobsim"
	"github.com/luci/luci-go/dm/appengine/mutate"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/prpc"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/tumble"
)

func init() {
	r := router.New()

	distributors := distributor.FactoryMap{}
	jobsim.AddFactory(distributors)
	reg := distributor.NewRegistry(distributors, mutate.FinishExecutionFn)

	tmb := tumble.Service{
		Middleware: func(c context.Context) context.Context {
			return distributor.WithRegistry(c, reg)
		},
	}

	basemw := gaemiddleware.BaseProd()

	distributor.InstallHandlers(reg, r, basemw)

	svr := prpc.Server{}
	deps.RegisterDepsServer(&svr, reg)
	discovery.Enable(&svr)
	svr.InstallHandlers(r, basemw)
	tmb.InstallHandlers(r)
	gaemiddleware.InstallHandlers(r, basemw)

	http.Handle("/", r)
}
