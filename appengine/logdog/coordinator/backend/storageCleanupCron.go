// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

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

			taskC <- createStorageCleanupTask(ls)
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
