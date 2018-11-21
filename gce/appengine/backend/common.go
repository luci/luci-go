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

	"go.chromium.org/luci/appengine/tq"
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

// srvKey is the key to a config.ConfigServer in the context.
var srvKey = "srv"

// withServer returns a new context with the given config.ConfigServer installed.
func withServer(c context.Context, srv config.ConfigServer) context.Context {
	return context.WithValue(c, &srvKey, srv)
}

// getServer returns the config.ConfigServer installed in the current context.
func getServer(c context.Context) config.ConfigServer {
	return c.Value(&srvKey).(config.ConfigServer)
}

// registerTasks registers task handlers with the given *tq.Dispatcher.
func registerTasks(dsp *tq.Dispatcher) {
	dsp.RegisterTask(&tasks.Ensure{}, ensure, ensureQueue, nil)
	dsp.RegisterTask(&tasks.Expand{}, expand, expandQueue, nil)
}

// InstallHandlers installs HTTP request handlers into the given router.
func InstallHandlers(r *router.Router, mw router.MiddlewareChain) {
	dsp := &tq.Dispatcher{}
	registerTasks(dsp)
	mw = mw.Extend(func(c *router.Context, next router.Handler) {
		c.Context = withServer(withDispatcher(c.Context, dsp), &rpc.Config{})
		next(c)
	})
	dsp.InstallRoutes(r, mw)
	r.GET("/internal/cron/process-config", mw, processHandler)
}
