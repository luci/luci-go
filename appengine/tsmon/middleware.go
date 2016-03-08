// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	gaeauth "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
)

var (
	initializeOnce sync.Once
	lastFlushed    = struct {
		time.Time
		sync.Mutex
	}{}
)

// Middleware returns a middleware that must be inserted into the chain to
// enable tsmon metrics to be sent on App Engine.
func Middleware(h middleware.Handler) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		initializeOnce.Do(func() {
			if err := initialize(c); err != nil {
				logging.Errorf(c, "Failed to initialize tsmon: %s", err)
				// Don't fail the request.
			}
		})
		h(c, rw, r, p)
		flushIfNeeded(c)
	}
}

func initialize(ctx context.Context) error {
	var mon monitor.Monitor
	i := info.Get(ctx)
	if i.IsDevAppServer() {
		mon = monitor.NewDebugMonitor("")
	} else {
		client := func(ctx context.Context) (*http.Client, error) {
			// Create an HTTP client with the default appengine service account.
			auth, err := gaeauth.Authenticator(ctx, pubsub.PublisherScopes, nil)
			if err != nil {
				return nil, err
			}
			return auth.Client()
		}

		var err error
		mon, err = monitor.NewPubsubMonitor(client, pubsubProject, pubsubTopic)
		if err != nil {
			return err
		}
	}

	// Create the target.
	tar := &target.Task{
		DataCenter:  proto.String(targetDataCenter),
		ServiceName: proto.String(i.AppID()),
		JobName:     proto.String(i.ModuleName()),
		HostName:    proto.String(i.VersionID()),
		TaskNum:     proto.Int32(-1),
	}

	tsmon.Initialize(mon, store.NewInMemory(tar))
	return nil
}

func flushIfNeeded(c context.Context) {
	if !updateLastFlushed(c) {
		return
	}

	if err := updateInstanceEntityAndFlush(c); err != nil {
		logging.Errorf(c, "Failed to flush tsmon metrics: %s", err)
	}
}

func updateLastFlushed(c context.Context) bool {
	now := clock.Now(c)
	minuteAgo := now.Add(-time.Minute)

	lastFlushed.Lock()
	defer lastFlushed.Unlock()

	if lastFlushed.After(minuteAgo) {
		return false
	}
	lastFlushed.Time = now // Don't hammer the datastore if task_num is not yet assigned.
	return true
}

func updateInstanceEntityAndFlush(c context.Context) error {
	c = info.Get(c).MustNamespace(instanceNamespace)

	task, ok := tsmon.Store().DefaultTarget().(*target.Task)
	if !ok {
		// tsmon probably failed to initialize - just do nothing.
		return fmt.Errorf("default tsmon target is not a Task: %v", tsmon.Store().DefaultTarget())
	}

	logger := logging.Get(c)
	entity := getOrCreateInstanceEntity(c)
	now := clock.Now(c)

	if entity.TaskNum < 0 {
		if *task.TaskNum >= 0 {
			// We used to have a task number but we don't any more (we were inactive
			// for too long), so clear our state.
			logging.Warningf(c, "Instance %s got purged from Datastore, but is still alive. "+
				"Clearing cumulative metrics", info.Get(c).InstanceID())
			tsmon.ResetCumulativeMetrics(c)
		}
		task.TaskNum = proto.Int32(-1)
		lastFlushed.Time = entity.LastUpdated

		// Start complaining if we haven't been given a task number after some time.
		shouldHaveTaskNumBy := entity.LastUpdated.Add(instanceExpectedToHaveTaskNum)
		if shouldHaveTaskNumBy.Before(now) {
			logger.Warningf("Instance %s is %s old with no task_num.",
				info.Get(c).InstanceID(), now.Sub(shouldHaveTaskNumBy).String())
		}
		return nil
	}

	task.TaskNum = proto.Int32(int32(entity.TaskNum))
	tsmon.Store().SetDefaultTarget(task)

	// Update the instance entity and put it back in the datastore asynchronously.
	entity.LastUpdated = now
	putDone := make(chan struct{})
	go func() {
		defer close(putDone)
		if err := datastore.Get(c).Put(entity); err != nil {
			logger.Errorf("Failed to update instance entity: %s", err)
		}
	}()

	ret := tsmon.Flush(c)

	<-putDone
	return ret
}
