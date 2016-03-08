// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/internal/logdog/archivist"
	"github.com/luci/luci-go/server/internal/logdog/service"
	"github.com/luci/luci-go/server/taskqueueClient"
	"golang.org/x/net/context"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

// application is the Archivist application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runArchivist(c context.Context) error {
	cfg := a.Config()

	coordCfg, acfg := cfg.GetCoordinator(), cfg.GetArchivist()
	switch {
	case coordCfg == nil:
		fallthrough
	case coordCfg.Project == "":
		return errors.New("missing coordinator project name")
	case coordCfg.ArchiveTaskQueue == "":
		return errors.New("missing archive task queue name")

	case acfg == nil:
		return errors.New("missing Archivist configuration")
	case acfg.GsBase == "":
		return errors.New("missing archive GS bucket")
	}

	// Construct and validate our GS base.
	gsBase := gs.Path(acfg.GsBase)
	if gsBase.Bucket() == "" {
		log.Fields{
			"gsBase": acfg.GsBase,
		}.Errorf(c, "Google Storage base does not include a bucket name.")
		return errors.New("invalid Google Storage base")
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

	// Initialize our Google Storage client.
	gsClient, err := a.GSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get Google Storage client.")
		return err
	}
	defer gsClient.Close()

	// Application shutdown will now operate by cancelling the Collector's
	// shutdown Context.
	shutdownCtx, shutdownFunc := context.WithCancel(c)
	a.SetShutdownFunc(shutdownFunc)

	ar := archivist.Archivist{
		Service:  a.Coordinator(),
		Storage:  s,
		GSClient: gsClient,

		GSBase:           gsBase,
		StreamIndexRange: int(acfg.StreamIndexRange),
		PrefixIndexRange: int(acfg.PrefixIndexRange),
		ByteRange:        int(acfg.ByteRange),
	}

	tqOpts := taskqueueClient.Options{
		Project:   coordCfg.Project,
		Queue:     coordCfg.ArchiveTaskQueue,
		Client:    tqClient,
		UserAgent: "LogDog Archivist",
		Tasks:     int(acfg.Tasks),
	}

	log.Fields{
		"project": tqOpts.Project,
		"queue":   tqOpts.Queue,
	}.Infof(c, "Pulling tasks from task queue.")
	taskqueueClient.RunTasks(shutdownCtx, tqOpts, func(c context.Context, t taskqueueClient.Task) bool {
		c = log.SetField(c, "taskID", t.ID)

		startTime := clock.Now(c)
		err := ar.ArchiveTask(c, t.Payload)
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

	log.Debugf(c, "Archivist finished.")
	return nil
}

// Entry point.
func main() {
	a := application{}
	a.Run(context.Background(), a.runArchivist)
}
