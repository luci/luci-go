// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"
)

// archiveTaskQueueName returns the task queue name for archival, or an error
// if it's not configured.
func archiveTaskQueueName(cfg *svcconfig.Config) (string, error) {
	q := cfg.GetCoordinator().ArchiveTaskQueue
	if q == "" {
		return "", errors.New("missing archive task queue name")
	}
	return q, nil
}

// createArchiveTask creates a new archive Task.
func createArchiveTask(cfg *svcconfig.Coordinator, ls *coordinator.LogStream, complete bool) (*tq.Task, error) {
	desc := logdog.ArchiveTask{
		Path:     string(ls.Path()),
		Complete: complete,
	}
	t, err := createPullTask(&desc)
	if err != nil {
		return nil, err
	}

	t.Name = fmt.Sprintf("archive-%s", ls.HashID())
	return t, nil
}

// HandleArchiveCron is the handler for the archive cron endpoint. This scans
// for terminal log streams that are ready for archival.
//
// This will be called periodically by AppEngine cron.
func (b *Backend) HandleArchiveCron(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.archiveCron(c, true)
	})
}

// HandleArchiveCronNT is the handler for the archive non-terminal cron
// endpoint. This scans for non-terminal log streams that have not been updated
// in sufficiently long that we're willing to declare them complete and mark
// them terminal.
//
// This will be called periodically by AppEngine cron.
func (b *Backend) HandleArchiveCronNT(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.archiveCron(c, false)
	})
}

// HandleArchiveCronPurge purges all archival tasks from the task queue.
func (b *Backend) HandleArchiveCronPurge(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		cfg, err := config.Load(c)
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to load configuration.")
			return err
		}

		queueName, err := archiveTaskQueueName(cfg)
		if err != nil {
			log.Errorf(c, "Failed to get task queue name.")
			return err
		}

		if err := tq.Get(c).Purge(queueName); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"queue":      queueName,
			}.Errorf(c, "Failed to purge task queue.")
			return err
		}
		return nil
	})
}

func (b *Backend) archiveCron(c context.Context, complete bool) error {
	cfg, err := config.Load(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load configuration.")
		return err
	}

	queueName, err := archiveTaskQueueName(cfg)
	if err != nil {
		log.Errorf(c, "Failed to get task queue name.")
		return err
	}

	now := clock.Now(c).UTC()
	q := ds.NewQuery("LogStream")

	var threshold time.Duration
	if complete {
		threshold = cfg.GetCoordinator().ArchiveDelay.Duration()
		q = q.Eq("State", coordinator.LSTerminated)
	} else {
		threshold = cfg.GetCoordinator().ArchiveDelayMax.Duration()
		q = q.Eq("State", coordinator.LSPending)
	}
	q = q.Lte("Updated", now.Add(-threshold))

	// Query and dispatch our tasks.
	var ierr error
	count, err := b.multiTask(c, queueName, func(taskC chan<- *tq.Task) {
		ierr = ds.Get(c).Run(q, func(ls *coordinator.LogStream) error {
			log.Fields{
				"id":      ls.HashID(),
				"updated": ls.Updated.String(),
			}.Infof(c, "Identified log stream ready for archival.")

			// If the log stream has a terminal index, and its Updated time is less than
			// the maximum archive delay, require this archival to be complete (no
			// missing LogEntry).
			//
			// If we're past maximum archive delay, settle for any (even empty) archival.
			// This is a failsafe to prevent logs from sitting in limbo forever.
			maxDelay := cfg.GetCoordinator().ArchiveDelayMax.Duration()
			requireComplete := !now.After(ls.Updated.Add(maxDelay))
			if !requireComplete {
				log.Fields{
					"path":             ls.Path(),
					"updatedTimestamp": ls.Updated,
					"maxDelay":         maxDelay,
				}.Warningf(c, "Log stream is past maximum archival delay. Dropping completeness requirement.")
			}

			task, err := createArchiveTask(cfg.GetCoordinator(), ls, requireComplete)
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"path":       ls.Path(),
				}.Errorf(c, "Failed to create archive task.")
				return err
			}

			taskC <- task
			return nil
		})
	})
	if err != nil || ierr != nil {
		log.Fields{
			log.ErrorKey: err,
			"queryErr":   ierr,
			"taskCount":  count,
		}.Errorf(c, "Failed to dispatch archival tasks.")
		return errors.New("failed to dispatch archival tasks")
	}

	log.Fields{
		"taskCount": count,
	}.Debugf(c, "Archive sweep completed successfully.")
	return nil
}
