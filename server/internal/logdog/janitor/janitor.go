// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package janitor

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Janitor is a stateless configuration capable of archiving individual log
// streams.
type Janitor struct {
	// Service is the client to use to communicate with Coordinator's Services
	// endpoint.
	Service logdog.ServicesClient

	// Storage is the intermediate storage instance to use to pull log entries for
	// archival.
	Storage storage.Storage
}

// storageBufferSize is the size, in bytes, of the LogEntry buffer that is used
// to during archival. This should be greater than the maximum LogEntry size.
const storageBufferSize = types.MaxLogEntryDataSize * 64

// CleanupTask processes and executes a single log stream cleanup task.
func (a *Janitor) CleanupTask(c context.Context, desc []byte) error {
	var task logdog.CleanupTask
	if err := proto.Unmarshal(desc, &task); err != nil {
		log.WithError(err).Errorf(c, "Failed to decode cleanup task.")
		return err
	}
	return a.Cleanup(c, &task)
}

// Cleanup cleans up logs from intermediate storage.
//
// The returned error may be wrapped in errors.Transient if it is believed to
// have been caused by a transient failure.
//
// If the supplied Context is Done, operation may terminate before completion,
// returning the Context's error.
func (a *Janitor) Cleanup(c context.Context, t *logdog.CleanupTask) error {
	// Validate the log stream path. We do that here because we need to ensure
	// that the path is valid prior to passing it to Storage, so we might as well
	// do it before our LoadStream RPC.
	path := types.StreamPath(t.Path)
	if err := path.Validate(); err != nil {
		log.WithError(err).Errorf(c, "Invalid log stream path.")
		return errors.New("invalid log stream path")
	}

	// Load the log stream's current state. We want to make sure that the log
	// stream's intermediate storage is not still needed, even if somehow a task
	// was dispatched.
	//
	// We will not cleanup the logs unless they are archived.
	//
	// NOTE: In the future, when purging is fully implemented, we will also allow
	// cleanup of non-archived log streams if they have been marked for deletion,
	// and we may also task the Janitor with deleting purged log stream archives.
	ls, err := a.Service.LoadStream(c, &logdog.LoadStreamRequest{
		Path: string(path),
	})
	switch {
	case err != nil:
		log.WithError(err).Errorf(c, "Failed to load log stream.")
		return err
	case ls.State == nil:
		return errors.New("missing state")
	case !ls.State.Archived:
		return errors.New("log stream is not archived")
	}

	// Purge the stream from storage.
	if err := a.Storage.Purge(path); err != nil {
		log.WithError(err).Errorf(c, "Failed to purge log stream path.")
		return err
	}

	// Mark the stream as cleaned up.
	_, err = a.Service.CleanupStream(c, &logdog.CleanupStreamRequest{
		Path: string(path),
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to mark stream as cleaned up.")
		return err
	}
	return nil
}
