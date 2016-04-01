// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/filter/dsQueryBatch"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"
)

const archiveTaskVersion = "v1"

// archiveTaskQueueName returns the task queue name for archival, or an error
// if it's not configured.
func archiveTaskQueueName(cfg *svcconfig.Config) (string, error) {
	q := cfg.GetCoordinator().ArchiveTaskQueue
	if q == "" {
		return "", errors.New("missing archive task queue name")
	}
	return q, nil
}

func archiveTaskNameForHash(hashID string) string {
	return fmt.Sprintf("archive-%s-%s", hashID, archiveTaskVersion)
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

	t.Name = archiveTaskNameForHash(ls.HashID())
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

	// If the log stream has a terminal index, and its Updated time is less than
	// the maximum archive delay, require this archival to be complete (no
	// missing LogEntry).
	//
	// If we're past maximum archive delay, settle for any (even empty) archival.
	// This is a failsafe to prevent logs from sitting in limbo forever.
	maxDelay := cfg.GetCoordinator().ArchiveDelayMax.Duration()

	// Perform a query, dispatching tasks in batches.
	batch := b.getMultiTaskBatchSize()

	ti := tq.Get(c)
	tasks := make([]*tq.Task, 0, batch)
	totalScheduledTasks := 0
	addAndMaybeDispatchTasks := func(task *tq.Task) error {
		switch task {
		case nil:
			if len(tasks) == 0 {
				return nil
			}

		default:
			tasks = append(tasks, task)
			if len(tasks) < batch {
				return nil
			}
		}

		err := ti.AddMulti(tasks, queueName)
		if merr, ok := err.(errors.MultiError); ok {
			for _, e := range merr {
				log.Warningf(c, "Task add error: %v", e)
			}
		}
		if err := errors.Filter(err, tq.ErrTaskAlreadyAdded); err != nil {
			log.Fields{
				log.ErrorKey:         err,
				"queue":              queueName,
				"numTasks":           len(tasks),
				"scheduledTaskCount": totalScheduledTasks,
			}.Errorf(c, "Failed to add tasks to task queue.")
			return errors.New("failed to add tasks to task queue")
		}

		totalScheduledTasks += len(tasks)
		tasks = tasks[:0]
		return nil
	}

	err = ds.Get(dsQueryBatch.BatchQueries(c, int32(batch))).Run(q, func(ls *coordinator.LogStream) error {
		requireComplete := !now.After(ls.Updated.Add(maxDelay))
		if !requireComplete {
			log.Fields{
				"path":             ls.Path(),
				"id":               ls.HashID(),
				"updatedTimestamp": ls.Updated,
				"maxDelay":         maxDelay,
			}.Warningf(c, "Identified log stream past maximum archival delay.")
		} else {
			log.Fields{
				"id":      ls.HashID(),
				"updated": ls.Updated.String(),
			}.Infof(c, "Identified log stream ready for archival.")
		}

		task, err := createArchiveTask(cfg.GetCoordinator(), ls, requireComplete)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       ls.Path(),
			}.Errorf(c, "Failed to create archive task.")
			return err
		}

		return addAndMaybeDispatchTasks(task)
	})
	if err != nil {
		log.Fields{
			log.ErrorKey:         err,
			"scheduledTaskCount": totalScheduledTasks,
		}.Errorf(c, "Outer archive query failed.")
		return errors.New("outer archive query failed")
	}

	// Dispatch any remaining enqueued tasks.
	if err := addAndMaybeDispatchTasks(nil); err != nil {
		return err
	}

	log.Fields{
		"scheduledTaskCount": totalScheduledTasks,
	}.Debugf(c, "Archive sweep completed successfully.")
	return nil
}
