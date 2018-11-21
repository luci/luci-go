// Copyright 2018 The LUCI Authors.
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

// Package backend includes cron and task queue handlers.
package backend

import (
	"context"
	"net/http"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/rpc"
)

// dspKey is the key to a *tq.Dispatcher in the context.
var dspKey = "dsp"

// withDispatcher returns a new context with the given *tq.Dispatcher installed.
func withDispatcher(c context.Context, dsp *tq.Dispatcher) context.Context {
	return context.WithValue(c, &dspKey, dsp)
}

// getDispatcher returns the *tq.Dispatcher installed in the current context.
func getDispatcher(c context.Context) *tq.Dispatcher {
	return c.Value(&dspKey).(*tq.Dispatcher)
}

// registerTasks registers task handlers with the given *tq.Dispatcher.
func registerTasks(dsp *tq.Dispatcher) {
	dsp.RegisterTask(&tasks.Create{}, create, createQueue, nil)
	dsp.RegisterTask(&tasks.Ensure{}, ensure, ensureQueue, nil)
	dsp.RegisterTask(&tasks.Expand{}, expand, expandQueue, nil)
}

// cfgKey is the key to a config.ConfigServer in the context.
var cfgKey = "cfg"

// withConfig returns a new context with the given config.ConfigServer installed.
func withConfig(c context.Context, cfg config.ConfigServer) context.Context {
	return context.WithValue(c, &cfgKey, cfg)
}

// getConfig returns the config.ConfigServer installed in the current context.
func getConfig(c context.Context) config.ConfigServer {
	return c.Value(&cfgKey).(config.ConfigServer)
}

// gceKey is the key to a *compute.Service in the context.
var gceKey = "gce"

// withCompute returns a new context with the given *compute.Service installed.
func withCompute(c context.Context, gce *compute.Service) context.Context {
	return context.WithValue(c, &gceKey, gce)
}

// getCompute returns the *compute.Service installed in the current context.
func getCompute(c context.Context) *compute.Service {
	return c.Value(&gceKey).(*compute.Service)
}

// newCompute returns a new *compute.Service. Panics on error.
func newCompute(c context.Context) *compute.Service {
	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(compute.ComputeScope))
	if err != nil {
		panic(err)
	}
	gce, err := compute.New(&http.Client{Transport: t})
	if err != nil {
		panic(err)
	}
	return gce
}

// InstallHandlers installs HTTP request handlers into the given router.
func InstallHandlers(r *router.Router, mw router.MiddlewareChain) {
	dsp := &tq.Dispatcher{}
	registerTasks(dsp)
	mw = mw.Extend(func(c *router.Context, next router.Handler) {
		// Install the task queue dispatcher, VMs config service, and GCE service.
		c.Context = withDispatcher(c.Context, dsp)
		c.Context = withConfig(c.Context, &rpc.Config{})
		c.Context = withCompute(c.Context, newCompute(c.Context))
		next(c)
	})
	dsp.InstallRoutes(r, mw)
	r.GET("/internal/cron/create-instances", mw, newHTTPHandler(createInstances))
	r.GET("/internal/cron/expand-vms", mw, newHTTPHandler(expandVMs))
}
