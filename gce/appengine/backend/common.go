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

	computealpha "google.golang.org/api/compute/v0.alpha"
	compute "google.golang.org/api/compute/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// Operation is a wrapper type over operation results in alpha and stable GCP operations.
type Operation struct {
	Stable *compute.Operation
	Alpha  *computealpha.Operation
}

// CommonOpError exposes just the subset of operation errors that are used
type CommonOpError struct {
	Code    string
	Message string
}

// GetErrors gets the errors for a stable or alpha Operation.
func (o Operation) GetErrors() []CommonOpError {
	switch {
	case o.Stable != nil:
		if o.Stable.Error == nil {
			return nil
		}
		errs := make([]CommonOpError, 0, len(o.Stable.Error.Errors))
		for _, err := range o.Stable.Error.Errors {
			errs = append(errs, CommonOpError{
				Code:    err.Code,
				Message: err.Message,
			})
		}
		return errs
	case o.Alpha != nil:
		if o.Alpha.Error == nil {
			return nil
		}
		errs := make([]CommonOpError, 0, len(o.Alpha.Error.Errors))
		for _, err := range o.Alpha.Error.Errors {
			errs = append(errs, CommonOpError{
				Code:    err.Code,
				Message: err.Message,
			})
		}
		return errs
	}
	return nil
}

// GetStatus gets the status for a stable or alpha operation.
func (o Operation) GetStatus() string {
	switch {
	case o.Stable != nil:
		return o.Stable.Status
	case o.Alpha != nil:
		return o.Alpha.Status
	}
	return ""
}

// ComputeService is a wrapper over a stable or alpha compute service.
type ComputeService struct {
	Stable *compute.Service
	Alpha  *computealpha.Service
}

// InsertInstance inserts a stable or beta compute instance, used to create instances that might use alpha features or might not.
func (c ComputeService) InsertInstance(ctx context.Context, project string, zone string, instance model.ComputeInstance, requestID string) (Operation, error) {
	switch {
	case instance.Stable != nil:
		call := c.Stable.Instances.Insert(project, zone, instance.Stable)
		stable, err := call.RequestId(requestID).Context(ctx).Do()
		return Operation{Stable: stable}, err
	default:
		call := c.Alpha.Instances.Insert(project, zone, instance.Alpha)
		alpha, err := call.RequestId(requestID).Context(ctx).Do()
		return Operation{Alpha: alpha}, err
	}
}

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
	dsp.RegisterTask(&tasks.ExpandConfig{}, expandConfig, expandConfigQueue, nil)
	dsp.RegisterTask(&tasks.ManageBot{}, manageBot, manageBotQueue, nil)
	dsp.RegisterTask(&tasks.ReportQuota{}, reportQuota, reportQuotaQueue, nil)
	dsp.RegisterTask(&tasks.TerminateBot{}, terminateBot, terminateBotQueue, nil)
	dsp.RegisterTask(&tasks.AuditProject{}, auditInstanceInZone, auditInstancesQueue, nil)
	dsp.RegisterTask(&tasks.DrainVM{}, drainVMQueueHandler, drainVMQueue, nil)
	dsp.RegisterTask(&tasks.InspectSwarming{}, inspectSwarming, inspectSwarmingQueue, nil)
	dsp.RegisterTask(&tasks.DeleteStaleSwarmingBot{}, deleteStaleSwarmingBot, deleteStaleSwarmingBotQueue, nil)
}

// gceKey is the key to a *compute.Service in the context.
var gceKey = "gce"

// withCompute returns a new context with the given *compute.Service installed.
func withCompute(c context.Context, gce ComputeService) context.Context {
	return context.WithValue(c, &gceKey, gce)
}

// getCompute returns the ComputeService installed in the current context.
func getCompute(c context.Context) ComputeService {
	return c.Value(&gceKey).(ComputeService)
}

// newCompute returns a new ComputeService. Panics on error.
func newCompute(c context.Context) ComputeService {
	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(compute.ComputeScope))
	if err != nil {
		panic(err)
	}
	stable, err := compute.New(&http.Client{Transport: t})
	if err != nil {
		panic(err)
	}
	alpha, err := computealpha.New(&http.Client{Transport: t})
	if err != nil {
		panic(err)
	}
	return ComputeService{
		Stable: stable,
		Alpha:  alpha,
	}
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		ctx = withDispatcher(ctx, dsp)
		ctx = withCompute(ctx, newCompute(ctx))
		ctx = withSwarming(ctx, newSwarming(ctx))
		c.Request = c.Request.WithContext(ctx)
		next(c)
	})
	dsp.InstallRoutes(r, mw)
	r.GET("/internal/cron/count-tasks", mw, newHTTPHandler(countTasks))
	r.GET("/internal/cron/count-vms", mw, newHTTPHandler(countVMsAsync))
	r.GET("/internal/cron/create-instances", mw, newHTTPHandler(createInstancesAsync))
	r.GET("/internal/cron/expand-configs", mw, newHTTPHandler(expandConfigsAsync))
	r.GET("/internal/cron/manage-bots", mw, newHTTPHandler(manageBotsAsync))
	r.GET("/internal/cron/report-quota", mw, newHTTPHandler(reportQuotasAsync))
	r.GET("/internal/cron/audit-project", mw, newHTTPHandler(auditInstances))
	r.GET("/internal/cron/drain-vms", mw, newHTTPHandler(drainVMsAsync))
	r.GET("/internal/cron/inspect-swarming", mw, newHTTPHandler(inspectSwarmingAsync))
}
