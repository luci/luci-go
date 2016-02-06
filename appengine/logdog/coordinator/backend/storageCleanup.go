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
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var errStorageCleanupFailed = errors.New("archive failed")

func createStorageCleanupTask(ls *coordinator.LogStream) *tq.Task {
	t := createTask("/archive/cleanup", map[string]string{
		"path": string(ls.Path()),
	})
	t.Name = fmt.Sprintf("cleanup-%s", ls.HashID())
	return t
}

// HandleStorageCleanup is a task queue endpoint that schedules a log stream to
// be purged from intermediate storage.
func (b *Backend) HandleStorageCleanup(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.handleStorageCleanupTask(c, r)
	})
}

func (b *Backend) handleStorageCleanupTask(c context.Context, r *http.Request) error {
	// Get the stream path from our task form. If the path is invalid, log the
	// error, but return nil since the task is junk.
	path := types.StreamPath(r.FormValue("path"))
	if err := path.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       path,
		}.Errorf(c, "Invalid stream path.")
		return nil
	}
	log.Fields{
		"path": path,
	}.Infof(c, "Received storage cleanup request.")

	st, err := b.s.Storage(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get Storage instance.")
		return errStorageCleanupFailed
	}
	defer st.Close()

	if err := st.Purge(path); err != nil {
		log.WithError(err).Errorf(c, "Failed to purge log stream path.")
		return errStorageCleanupFailed
	}

	// Update the log stream's status.
	now := clock.Get(c).Now()
	ls := coordinator.LogStreamFromPath(path)
	ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)
		if err := di.Get(ls); err != nil {
			return err
		}
		if ls.State == coordinator.LSDone {
			log.Warningf(c, "Log stream already marked as done.")
			return nil
		}

		ls.Updated = now
		ls.State = coordinator.LSDone
		if err := ls.Put(di); err != nil {
			log.WithError(err).Errorf(c, "Failed to update log stream.")
			return err
		}
		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to mark log stream as cleaned up.")
		return err
	}
	return nil
}
