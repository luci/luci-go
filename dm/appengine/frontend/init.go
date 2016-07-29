// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"os"

	"golang.org/x/net/context"

	"google.golang.org/appengine"

	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/filesystem"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/appengine/deps"
	"github.com/luci/luci-go/dm/appengine/distributor"
	"github.com/luci/luci-go/dm/appengine/distributor/jobsim"
	"github.com/luci/luci-go/dm/appengine/mutate"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/prpc"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/tumble"
)

func addConfigProd(c context.Context) context.Context {
	cfg, err := gaeconfig.New(c)
	switch err {
	case nil:
		c = config.SetImplementation(c, cfg)
	case gaeconfig.ErrNotConfigured:
		logging.Warningf(c, "luci-config service url not configured. Configure this at /admin/settings/gaeconfig.")
		fallthrough
	default:
		panic(err)
	}
	return c
}

func baseProd() router.MiddlewareChain {
	return gaemiddleware.BaseProd().Extend(
		func(c *router.Context, next router.Handler) {
			c.Context = addConfigProd(c.Context)
			next(c)
		},
	)
}

func addConfigDev(c context.Context) context.Context {
	fpath := os.Getenv("LUCI_DM_CONFIG_BASE_PATH")
	if fpath == "" {
		panic(fmt.Errorf("LUCI_DM_CONFIG_BASE_PATH must be set in the environment"))
	}
	fs, err := filesystem.New(fpath)
	if err != nil {
		panic(fmt.Errorf("while setting up LUCI_DM_CONFIG_BASE_PATH: %s", err))
	}
	return config.SetImplementation(c, fs)
}

func baseDev() router.MiddlewareChain {
	return gaemiddleware.BaseProd().Extend(
		func(c *router.Context, next router.Handler) {
			c.Context = addConfigDev(c.Context)
			next(c)
		},
	)
}

func init() {
	r := router.New()
	tmb := tumble.Service{}

	distributors := distributor.FactoryMap{}
	jobsim.AddFactory(distributors)
	reg := distributor.NewRegistry(distributors, mutate.FinishExecutionFn)

	basemw := baseProd()
	tmb.Middleware = func(c context.Context) context.Context {
		return distributor.WithRegistry(addConfigProd(c), reg)
	}
	if appengine.IsDevAppServer() {
		basemw = baseDev()
		tmb.Middleware = func(c context.Context) context.Context {
			return distributor.WithRegistry(addConfigDev(c), reg)
		}
	}

	distributor.InstallHandlers(reg, r, basemw)

	svr := prpc.Server{}
	deps.RegisterDepsServer(&svr, reg)
	discovery.Enable(&svr)
	svr.InstallHandlers(r, basemw)
	tmb.InstallHandlers(r)
	gaemiddleware.InstallHandlers(r, basemw)

	http.Handle("/", r)
}
