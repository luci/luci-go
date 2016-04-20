// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/filter/dsQueryBatch"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

const archiveTaskVersion = "v4"

// HandleArchiveCron is the handler for the archive cron endpoint. This scans
// for log streams that are ready for archival.
//
// This will be called periodically by AppEngine cron.
func (b *Backend) HandleArchiveCron(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.archiveCron(c)
	})
}

func (b *Backend) archiveCron(c context.Context) error {
	services := b.GetServices()
	_, cfg, err := services.Config(c)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %v", err)
	}

	archiveDelayMax := cfg.Coordinator.ArchiveDelayMax.Duration()
	if archiveDelayMax <= 0 {
		return fmt.Errorf("must have positive maximum archive delay, not %q", archiveDelayMax.String())
	}

	ap, err := services.ArchivalPublisher(c)
	if err != nil {
		return fmt.Errorf("failed to get archival publisher: %v", err)
	}

	threshold := clock.Now(c).UTC().Add(-archiveDelayMax)
	log.Fields{
		"threshold": threshold,
	}.Infof(c, "Querying for all streaming logs created before max archival threshold.")

	// Query for log streams that were created <= our threshold and that are
	// still in LSStreaming state.
	//
	// We order descending because this is already an index that we use for our
	// "logdog.Logs.Query".
	q := ds.NewQuery("LogStream").
		KeysOnly(true).
		Eq("State", coordinator.LSStreaming).
		Lte("Created", threshold).
		Order("-Created", "State")

	// Since these logs are beyond maximum archival delay, we will dispatch
	// archival immediately.
	params := coordinator.ArchivalParams{
		RequestID: info.Get(c).RequestID(),
	}

	// Create archive tasks for our expired log streams in parallel.
	batch := b.getMultiTaskBatchSize()
	var tasked int32
	var failed int32

	var ierr error
	parallel.Ignore(parallel.Run(batch, func(taskC chan<- func() error) {
		// Run a batched query across the expired log stream space.
		ierr = ds.Get(dsQueryBatch.BatchQueries(c, int32(batch))).Run(q, func(lsKey *ds.Key) error {
			var ls coordinator.LogStream
			ds.PopulateKey(&ls, lsKey)

			// Archive this log stream in a transaction.
			taskC <- func() error {
				err := ds.Get(c).RunInTransaction(func(c context.Context) error {
					if err := ds.Get(c).Get(&ls); err != nil {
						log.WithError(err).Errorf(c, "Failed to load stream.")
						return err
					}

					log.Fields{
						"path": ls.Path(),
						"id":   ls.HashID,
					}.Infof(c, "Identified expired log stream.")

					if err := params.PublishTask(c, ap, &ls); err != nil {
						if err == coordinator.ErrArchiveTasked {
							log.Warningf(c, "Archival has already been tasked for this stream.")
							return nil
						}
						return err
					}
					return ds.Get(c).Put(&ls)
				}, nil)

				if err != nil {
					log.Fields{
						log.ErrorKey: err,
						"path":       ls.Path(),
					}.Errorf(c, "Failed to archive log stream.")
					atomic.AddInt32(&failed, 1)
					return nil // Nothing will consume it anyway.
				}

				log.Fields{
					"path":         ls.Path(),
					"id":           ls.HashID,
					"archiveTopic": cfg.Coordinator.ArchiveTopic,
				}.Infof(c, "Created archive task.")
				atomic.AddInt32(&tasked, 1)
				return nil
			}

			return nil
		})
	}))

	// Return an error code if we experienced any failures. This doesn't really
	// have an impact, but it will show up as a "!" in the cron UI.
	switch {
	case ierr != nil:
		log.Fields{
			log.ErrorKey:   err,
			"archiveCount": tasked,
		}.Errorf(c, "Failed to execute expired tasks query.")
		return ierr

	case failed > 0:
		log.Fields{
			log.ErrorKey:   err,
			"archiveCount": tasked,
			"failCount":    failed,
		}.Errorf(c, "Failed to archive candidate all streams.")
		return errors.New("failed to archive all candidate streams")

	default:
		log.Fields{
			"archiveCount": tasked,
		}.Infof(c, "Archive sweep completed successfully.")
		return nil
	}
}
