// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/internal/logdog/janitor"
	"github.com/luci/luci-go/server/internal/logdog/service"
	"github.com/luci/luci-go/server/taskqueueClient"
	"golang.org/x/net/context"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

// application is the Janitor application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runJanitor(c context.Context) error {
	cfg := a.Config()

	coordCfg, jcfg := cfg.GetCoordinator(), cfg.GetJanitor()
	switch {
	case coordCfg == nil:
		fallthrough
	case coordCfg.Project == "":
		return errors.New("missing coordinator project name")
	case coordCfg.StorageCleanupTaskQueue == "":
		return errors.New("missing storage cleanup task queue name")
	}

	// Initialize task queue client.
	tqClient, err := a.AuthenticatedClient(func(o *auth.Options) {
		o.Scopes = taskqueueClient.Scopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get task queue client.")
		return err
	}

	// Initialize our Storage.
	s, err := a.IntermediateStorage(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get storage instance.")
		return err
	}
	defer s.Close()

	// Application shutdown will now operate by cancelling the Collector's
	// shutdown Context.
	shutdownCtx, shutdownFunc := context.WithCancel(c)
	a.SetShutdownFunc(shutdownFunc)

	j := janitor.Janitor{
		Service: a.Coordinator(),
		Storage: s,
	}

	tqOpts := taskqueueClient.Options{
		Project:   coordCfg.Project,
		Queue:     coordCfg.StorageCleanupTaskQueue,
		Client:    tqClient,
		UserAgent: "LogDog Janitor",
	}
	if jcfg != nil {
		tqOpts.Tasks = int(jcfg.Tasks)
	}

	log.Fields{
		"project": tqOpts.Project,
		"queue":   tqOpts.Queue,
	}.Infof(c, "Pulling tasks from task queue.")
	taskqueueClient.RunTasks(shutdownCtx, tqOpts, func(c context.Context, t taskqueueClient.Task) bool {
		c = log.SetField(c, "taskID", t.ID)

		startTime := clock.Now(c)
		err := j.CleanupTask(c, t.Payload)
		duration := clock.Now(c).Sub(startTime)

		switch {
		case errors.IsTransient(err):
			// Do not consume
			log.Fields{
				log.ErrorKey: err,
				"duration":   duration,
			}.Warningf(c, "TRANSIENT error processing task.")
			return false

		case err == nil:
			log.Fields{
				"duration": duration,
			}.Infof(c, "Task successfully processed; deleting.")
			return true

		default:
			log.Fields{
				log.ErrorKey: err,
				"duration":   duration,
			}.Errorf(c, "Non-transient error processing task; deleting.")
			return true
		}
	})

	log.Debugf(c, "Janitor finished.")
	return nil
}

// Entry point.
func main() {
	a := application{}
	a.Run(context.Background(), a.runJanitor)
}
