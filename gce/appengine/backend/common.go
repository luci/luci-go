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
	"time"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
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
	dsp.RegisterTask(&tasks.CountVMs{}, countVMs, countVMsQueue, nil)
	dsp.RegisterTask(&tasks.CreateInstance{}, createInstance, createInstanceQueue, nil)
	dsp.RegisterTask(&tasks.CreateVM{}, createVM, createVMQueue, nil)
	dsp.RegisterTask(&tasks.DeleteBot{}, deleteBot, deleteBotQueue, nil)
	dsp.RegisterTask(&tasks.DestroyInstance{}, destroyInstance, destroyInstanceQueue, nil)
	dsp.RegisterTask(&tasks.DrainVM{}, drainVM, drainVMQueue, nil)
	dsp.RegisterTask(&tasks.ExpandConfig{}, expandConfig, expandConfigQueue, nil)
	dsp.RegisterTask(&tasks.ManageBot{}, manageBot, manageBotQueue, nil)
	dsp.RegisterTask(&tasks.ReportQuota{}, reportQuota, reportQuotaQueue, nil)
	dsp.RegisterTask(&tasks.TerminateBot{}, terminateBot, terminateBotQueue, nil)
}

// cfgKey is the key to a config.ConfigurationServer in the context.
var cfgKey = "cfg"

// withConfig returns a new context with the given config.ConfigurationServer installed.
func withConfig(c context.Context, cfg config.ConfigurationServer) context.Context {
	return context.WithValue(c, &cfgKey, cfg)
}

// getConfig returns the config.ConfigurationServer installed in the current context.
func getConfig(c context.Context) config.ConfigurationServer {
	return c.Value(&cfgKey).(config.ConfigurationServer)
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

// swrKey is the key to a *swarming.Service in the context.
var swrKey = "swr"

// withSwarming returns a new context with the given *swarming.Service installed.
func withSwarming(c context.Context, swr *swarming.Service) context.Context {
	return context.WithValue(c, &swrKey, swr)
}

// getSwarming returns the *swarming.Service installed in the current context.
func getSwarming(c context.Context, url string) *swarming.Service {
	swr := c.Value(&swrKey).(*swarming.Service)
	swr.BasePath = url + "/_ah/api/swarming/v1/"
	return swr
}

// newSwarming returns a new *swarming.Service. Panics on error.
func newSwarming(c context.Context) *swarming.Service {
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		panic(err)
	}
	swr, err := swarming.New(&http.Client{Transport: t})
	if err != nil {
		panic(err)
	}
	return swr
}

// InstallHandlers installs HTTP request handlers into the given router.
func InstallHandlers(r *router.Router, mw router.MiddlewareChain) {
	dsp := &tq.Dispatcher{}
	registerTasks(dsp)
	mw = mw.Extend(func(c *router.Context, next router.Handler) {
		c.Context, _ = context.WithTimeout(c.Context, 30*time.Second)
		c.Context = withDispatcher(c.Context, dsp)
		c.Context = withConfig(c.Context, &rpc.Config{})
		c.Context = withCompute(c.Context, newCompute(c.Context))
		c.Context = withSwarming(c.Context, newSwarming(c.Context))
		next(c)
	})
	dsp.InstallRoutes(r, mw)
	r.GET("/internal/cron/count-vms", mw, newHTTPHandler(countVMsAsync))
	r.GET("/internal/cron/create-instances", mw, newHTTPHandler(createInstancesAsync))
	r.GET("/internal/cron/drain-vms", mw, newHTTPHandler(drainVMsAsync))
	r.GET("/internal/cron/expand-configs", mw, newHTTPHandler(expandConfigsAsync))
	r.GET("/internal/cron/manage-bots", mw, newHTTPHandler(manageBotsAsync))
	r.GET("/internal/cron/report-quota", mw, newHTTPHandler(reportQuotasAsync))
}
