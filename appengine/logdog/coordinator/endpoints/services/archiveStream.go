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

// ArchiveStream implements the logdog.ServicesServer interface.
func (b *Server) ArchiveStream(c context.Context, req *logdog.ArchiveStreamRequest) (*google.Empty, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	log.Fields{
		"path": req.Path,
	}.Infof(c, "Marking log stream as archived.")

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

	// (Non-transactional) Is the log stream already archived?
	switch err := ds.Get(c).Get(ls); err {
	case nil:
		if ls.Archived() {
			log.Infof(c, "Log stream already marked as archived (non-transactional).")
			return &google.Empty{}, nil
		}

	case ds.ErrNoSuchEntity:
		break

	default:
		log.WithError(err).Errorf(c, "Failed to check for log stream archvial state.")
		return nil, grpcutil.Internal
	}

	// Post the archival results to the Coordinator.
	now := clock.Now(c).UTC()
	err := ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)
		if err := di.Get(ls); err != nil {
			return err
		}
		if ls.Archived() {
			log.Infof(c, "Log stream already marked as archived.")
			return nil
		}

		// Update archival information. Make sure this actually marks the stream as
		// archived.
		ls.Updated = now
		ls.State = coordinator.LSArchived
		ls.ArchiveWhole = req.Complete
		ls.TerminalIndex = req.TerminalIndex
		ls.ArchiveStreamURL = req.StreamUrl
		ls.ArchiveStreamSize = req.StreamSize
		ls.ArchiveIndexURL = req.IndexUrl
		ls.ArchiveIndexSize = req.IndexSize
		ls.ArchiveDataURL = req.DataUrl
		ls.ArchiveDataSize = req.DataSize

		// Update the log stream.
		if err := ls.Put(di); err != nil {
			log.WithError(err).Errorf(c, "Failed to update log stream.")
			return err
		}

		log.Infof(c, "Successfully marked stream as archived.")
		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to mark stream as archived.")
		return nil, grpcutil.Internal
	}

	return &google.Empty{}, nil
}
