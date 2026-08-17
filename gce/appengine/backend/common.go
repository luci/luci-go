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
	"hash/fnv"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	computealpha "google.golang.org/api/compute/v0.alpha"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
	swarminggrpcpb "go.chromium.org/luci/swarming/proto/api_v2/grpcpb"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
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

// ManageBotQueues is the list of queues to distribute manage-bot tasks across for load balancing purposes.
var ManageBotQueues = []string{
	"manage-bot",
	"manage-bot-2",
}

func getManageBotQueue(id string) string {
	switch len(ManageBotQueues) {
	case 0:
		return ""
	case 1:
		return ManageBotQueues[0]
	default:
		h := fnv.New32a()
		h.Write([]byte(id))
		return ManageBotQueues[int(h.Sum32()%uint32(len(ManageBotQueues)))]
	}
}

// DeleteStaleSwarmingBotsQueues is the list of queues to distribute delete-stale-swarming-bots tasks across.
var DeleteStaleSwarmingBotsQueues = []string{
	deleteStaleSwarmingBotsQueue,
	deleteStaleSwarmingBots2Queue,
}

func getDeleteStaleSwarmingBotsQueue(id string) string {
	switch len(DeleteStaleSwarmingBotsQueues) {
	case 0:
		return ""
	case 1:
		return DeleteStaleSwarmingBotsQueues[0]
	default:
		h := fnv.New32a()
		h.Write([]byte(id))
		return DeleteStaleSwarmingBotsQueues[int(h.Sum32()%uint32(len(DeleteStaleSwarmingBotsQueues)))]
	}
}

// registerTasks registers task handlers with the given *tq.Dispatcher.
func registerTasks(dsp *tq.Dispatcher) {
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "count-vms",
		Prototype: &tasks.CountVMs{},
		Queue:     countVMsQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return countVMs(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "create-instance",
		Prototype: &tasks.CreateInstance{},
		Queue:     createInstanceQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return createInstance(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "create-vm",
		Prototype: &tasks.CreateVM{},
		Queue:     createVMQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return createVM(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "delete-bot",
		Prototype: &tasks.DeleteBot{},
		Queue:     deleteBotQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return deleteBot(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "destroy-instance",
		Prototype: &tasks.DestroyInstance{},
		Queue:     destroyInstanceQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return destroyInstance(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "expand-config",
		Prototype: &tasks.ExpandConfig{},
		Queue:     expandConfigQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return expandConfig(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "manage-bot",
		Prototype: &tasks.ManageBot{},
		QueuePicker: func(c context.Context, t *tq.Task) (string, error) {
			msg, ok := t.Payload.(*tasks.ManageBot)
			if !ok || msg.GetId() == "" {
				return manageBotQueue, nil
			}
			return getManageBotQueue(msg.GetId()), nil
		},
		Kind: tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return manageBot(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "report-quota",
		Prototype: &tasks.ReportQuota{},
		Queue:     reportQuotaQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return reportQuota(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "terminate-bot",
		Prototype: &tasks.TerminateBot{},
		Queue:     terminateBotQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return terminateBot(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "audit-instances",
		Prototype: &tasks.AuditProject{},
		Queue:     auditInstancesQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return auditInstanceInZone(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "drain-vm",
		Prototype: &tasks.DrainVM{},
		Queue:     drainVMQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return drainVMQueueHandler(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "inspect-swarming",
		Prototype: &tasks.InspectSwarming{},
		Queue:     inspectSwarmingQueue,
		Kind:      tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return inspectSwarming(c, payload)
		},
	})
	dsp.RegisterTaskClass(tq.TaskClass{
		ID:        "delete-stale-swarming-bots",
		Prototype: &tasks.DeleteStaleSwarmingBots{},
		QueuePicker: func(c context.Context, t *tq.Task) (string, error) {
			msg, ok := t.Payload.(*tasks.DeleteStaleSwarmingBots)
			if !ok || len(msg.GetBots()) == 0 || msg.GetBots()[0].GetId() == "" {
				return deleteStaleSwarmingBotsQueue, nil
			}
			return getDeleteStaleSwarmingBotsQueue(msg.GetBots()[0].GetId()), nil
		},
		Kind: tq.FollowsContext,
		Handler: func(c context.Context, payload proto.Message) error {
			return deleteStaleSwarmingBots(c, payload)
		},
	})
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

// swrKey is the key to swarmingFactory in the context.
var swrKey = "swr"

// swarmingFactroy produces Swarming client connected to the given server.
type swarmingFactory func(c context.Context, server string) swarminggrpcpb.BotsClient

// withSwarming returns a new context with the given swarming client factory.
func withSwarming(c context.Context, factory swarmingFactory) context.Context {
	return context.WithValue(c, &swrKey, factory)
}

// getSwarming returns the swarming client connected to the given server.
//
// Uses the factory in the context to construct it.
func getSwarming(c context.Context, url string) swarminggrpcpb.BotsClient {
	return c.Value(&swrKey).(swarmingFactory)(c, url)
}

// newSwarming produces a Swarming client connected to the given server.
//
// Panics on errors.
func newSwarming(c context.Context, url string) swarminggrpcpb.BotsClient {
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		panic(err)
	}
	return swarminggrpcpb.NewBotsClient(
		&prpc.Client{
			C:       &http.Client{Transport: t},
			Host:    strings.TrimPrefix(url, "https://"),
			Options: prpc.DefaultOptions(),
		},
	)
}

// InstallHandlers installs HTTP request handlers into the given router.
func InstallHandlers(r *router.Router, mw router.MiddlewareChain) {
	region := os.Getenv("GOOGLE_CLOUD_REGION")
	if region == "" {
		region = "us-central1"
	}
	dsp := &tq.Dispatcher{
		GAE:          true,
		CloudProject: os.Getenv("GOOGLE_CLOUD_PROJECT"),
		CloudRegion:  region,
	}
	dsp.Sweeper = tq.NewDistributedSweeper(dsp, tq.DistributedSweeperOptions{
		TaskQueue: "tq-sweep",
	})
	registerTasks(dsp)
	var (
		subMu sync.RWMutex
		sub   tq.Submitter
	)
	getSubmitter := func(ctx context.Context) tq.Submitter {
		subMu.RLock()
		s := sub
		subMu.RUnlock()
		if s != nil {
			return s
		}

		subMu.Lock()
		defer subMu.Unlock()
		if sub != nil {
			return sub
		}
		creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
		if err != nil {
			logging.Errorf(ctx, "failed to get RPC credentials for TQ submitter: %s", err)
			return nil
		}
		s, err = tq.NewCloudSubmitter(context.Background(), creds)
		if err != nil {
			logging.Errorf(ctx, "failed to create TQ CloudSubmitter: %s", err)
			return nil
		}
		sub = s
		return sub
	}

	mw = mw.Extend(func(c *router.Context, next router.Handler) {
		ctx := c.Request.Context()
		appID := info.AppID(ctx)
		if !info.IsDevAppServer(ctx) && appID != "" && appID != "app" && appID != "none" && appID != "testbed-test" {
			if s := getSubmitter(ctx); s != nil {
				ctx = tq.UseSubmitter(ctx, s)
			}
		}
		ctx = withDispatcher(ctx, dsp)
		ctx = withCompute(ctx, newCompute(ctx))
		ctx = withSwarming(ctx, newSwarming)
		c.Request = c.Request.WithContext(ctx)
		next(c)
	})
	taskMw := mw.Extend(func(c *router.Context, next router.Handler) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		next(c)
	})
	subR := r.Subrouter("/")
	subR.Use(taskMw)
	dsp.InstallTasksRoutes(subR, "/internal/tasks")
	dsp.InstallSweepRoute(subR, "/internal/tasks/c/sweep")
	cronMw := mw.Extend(gaemiddleware.RequireCron, func(c *router.Context, next router.Handler) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		next(c)
	})
	r.GET("/internal/cron/count-tasks", cronMw, newHTTPHandler(countTasks))
	r.GET("/internal/cron/count-vms", cronMw, newHTTPHandler(countVMsAsync))
	r.GET("/internal/cron/create-instances", cronMw, newHTTPHandler(createInstancesAsync))
	r.GET("/internal/cron/expand-configs", cronMw, newHTTPHandler(expandConfigsAsync))
	r.GET("/internal/cron/manage-bots", cronMw, newHTTPHandler(manageBotsAsync))
	r.GET("/internal/cron/report-quota", cronMw, newHTTPHandler(reportQuotasAsync))
	r.GET("/internal/cron/audit-project", cronMw, newHTTPHandler(auditInstances))
	r.GET("/internal/cron/drain-vms", cronMw, newHTTPHandler(drainVMsAsync))
	r.GET("/internal/cron/inspect-swarming", cronMw, newHTTPHandler(inspectSwarmingAsync))
	r.GET("/internal/cron/dump-datastore", cronMw, newHTTPHandler(dumpDatastoreSync))
}
