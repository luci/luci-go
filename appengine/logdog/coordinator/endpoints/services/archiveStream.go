// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func (b *server) ArchiveStream(c context.Context, req *logdog.ArchiveStreamRequest) (*google.Empty, error) {
	log.Fields{
		"project":       req.Project,
		"path":          req.Path,
		"complete":      req.Complete(),
		"terminalIndex": req.TerminalIndex,
		"logEntryCount": req.LogEntryCount,
		"error":         req.Error,
	}.Infof(c, "Received archival request.")

	// Verify that the request is minimially valid.
	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid log stream path: %v", err)
	}

	switch {
	case req.IndexUrl == "":
		return nil, grpcutil.Errf(codes.InvalidArgument, "missing required index archive URL")
	case req.StreamUrl == "":
		return nil, grpcutil.Errf(codes.InvalidArgument, "missing required stream archive URL")
	}

	ls := coordinator.LogStreamFromPath(path)

	log.Fields{
		"id": ls.ID,
	}.Infof(c, "Log stream ID.")

	// Post the archival results to the Coordinator.
	now := clock.Now(c).UTC()
	var ierr error
	err := ds.Get(c).RunInTransaction(func(c context.Context) error {
		ierr = nil

		// Note that within this transaction, we have two return values:
		// - Non-nil to abort the transaction.
		// - Specific error via "ierr".
		di := ds.Get(c)
		if err := di.Get(ls); err != nil {
			return err
		}

		// If our log stream is not in LSArchiveTasked, we will reject this archive
		// request with FailedPrecondition.
		switch {
		case ls.Archived():
			// Return nil if the log stream is already archived (idempotent).
			log.Warningf(c, "Log stream is already archived.")
			return nil

		case ls.State != coordinator.LSArchiveTasked:
			log.Fields{
				"state": ls.State,
			}.Errorf(c, "Log stream is not in archival tasked state.")
			ierr = grpcutil.Errf(codes.FailedPrecondition, "Log stream has not tasked an archival.")
			return ierr
		}

		// If this request contained an error, we will record an empty archival and
		// log a warning.
		if req.Error != "" {
			log.Fields{
				"archiveError": req.Error,
			}.Warningf(c, "Log stream archival indicated error. Archiving empty stream.")

			req.TerminalIndex = -1
			req.LogEntryCount = 0
		}

		// Update archival information. Make sure this actually marks the stream as
		// archived.
		ls.State = coordinator.LSArchived
		ls.ArchivedTime = now
		ls.ArchivalKey = nil // No point in wasting datastore space on this.

		if ls.TerminalIndex < 0 {
			// Also set the terminated time.
			ls.TerminatedTime = now
		}
		ls.TerminalIndex = req.TerminalIndex

		ls.ArchiveLogEntryCount = req.LogEntryCount
		ls.ArchiveStreamURL = req.StreamUrl
		ls.ArchiveStreamSize = req.StreamSize
		ls.ArchiveIndexURL = req.IndexUrl
		ls.ArchiveIndexSize = req.IndexSize
		ls.ArchiveDataURL = req.DataUrl
		ls.ArchiveDataSize = req.DataSize

		// Update the log stream.
		if err := di.Put(ls); err != nil {
			log.WithError(err).Errorf(c, "Failed to update log stream.")
			return err
		}

		log.Infof(c, "Successfully marked stream as archived.")
		return nil
	}, nil)
	if ierr != nil {
		log.WithError(ierr).Errorf(c, "Failed to mark stream as archived.")
		return nil, ierr
	}
	if err != nil {
		log.WithError(err).Errorf(c, "Internal error.")
		return nil, grpcutil.Internal
	}

	return &google.Empty{}, nil
}
