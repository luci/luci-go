// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
	"fmt"
	"net/http"

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

// storageCleanupTaskQueueName returns the task queue name for archival, or an error
// if it's not configured.
func storageCleanupTaskQueueName(cfg *svcconfig.Config) (string, error) {
	q := cfg.GetCoordinator().StorageCleanupTaskQueue
	if q == "" {
		return "", errors.New("missing storage cleanup task queue name")
	}
	return q, nil
}

// createStorageCleanupTask creates a new storage cleanup Task.
//
// If payload is true, the task's payload will be generated and included in the
// returned Task. This generation may fail and result in an error being
// returned. If false, the payload will be empty and no error can be returned.
func createStorageCleanupTask(ls *coordinator.LogStream) (*tq.Task, error) {
	desc := logdog.ArchiveTask{
		Path: string(ls.Path()),
	}
	t, err := createPullTask(&desc)
	if err != nil {
		return nil, err
	}

	t.Name = fmt.Sprintf("cleanup-%s", ls.HashID())
	return t, nil
}

// HandleStorageCleanupCron is the handler for the storageCleanup cron endpoint.
// This scans for log streams that have been archived but not cleaned up
// (LSDone). These logs have entries both in intermediate and archive storage,
// so they are safe to remove from intermediate storage.
//
// This will be called periodically by AppEngine cron.
func (b *Backend) HandleStorageCleanupCron(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.storageCleanupCron(c)
	})
}

func (b *Backend) storageCleanupCron(c context.Context) error {
	cfg, err := config.Load(c)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to load configuration.")
		return err
	}

	// Get the task queue name. Implicitly validates that cfg.Coordinator is not
	// nil.
	ccfg := cfg.GetCoordinator()
	switch {
	case ccfg == nil:
		return errors.New("no coordinator configuration")
	case ccfg.StorageCleanupTaskQueue == "":
		return errors.New("no storage cleanup task queue defined")
	}

	now := clock.Now(c).UTC()
	threshold := ccfg.StorageCleanupDelay.Duration()

	// Build a query for log streams that are candidates for cleanup.
	//
	// We include "Terminated" so we can re-use the index that archiveCron uses.
	q := ds.NewQuery("LogStream").
		Eq("State", coordinator.LSArchived).
		Lte("Updated", now.Add(-threshold))

	// Query and dispatch our tasks.
	var ierr error
	count, err := b.multiTask(c, ccfg.StorageCleanupTaskQueue, func(taskC chan<- *tq.Task) {
		ierr = ds.Get(c).Run(q, func(ls *coordinator.LogStream) error {
			log.Fields{
				"id":      ls.HashID(),
				"updated": ls.Updated.String(),
			}.Infof(c, "Identified log stream ready for storage cleanup.")

			task, err := createStorageCleanupTask(ls)
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"path":       ls.Path(),
				}.Errorf(c, "Failed to create storage cleanup task.")
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
		}.Errorf(c, "Failed to dispatch storage cleanup tasks.")
		return errors.New("failed to dispatch storage cleanup tasks")
	}

	log.Fields{
		"taskCount": count,
	}.Debugf(c, "Storage cleanup sweep completed successfully.")
	return nil
}
