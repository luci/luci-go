// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"

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

// CleanupStream implements the logdog.ServicesServer interface.
func (b *Server) CleanupStream(c context.Context, req *logdog.CleanupStreamRequest) (*google.Empty, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	log.Fields{
		"path": req.Path,
	}.Infof(c, "Marking log stream as cleaned up.")

	// Verify that the request is minimially valid.
	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid log stream path: %v", err)
	}

	ls := coordinator.LogStreamFromPath(path)

	// (Non-transactional) Is the log stream already cleaned up?
	switch err := ds.Get(c).Get(ls); err {
	case nil:
		if ls.State == coordinator.LSDone {
			log.Infof(c, "Log stream already marked as cleaned up (non-transactional).")
			return &google.Empty{}, nil
		}

	case ds.ErrNoSuchEntity:
		break

	default:
		log.WithError(err).Errorf(c, "Failed to check for log stream cleanup state.")
		return nil, grpcutil.Internal
	}

	// Mark the stream as cleaned up.
	now := clock.Now(c).UTC()
	err := ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)
		if err := di.Get(ls); err != nil {
			return err
		}
		switch {
		case ls.State == coordinator.LSDone:
			log.Infof(c, "Log stream is already marked as cleaned up.")
			return nil

		case !ls.Archived():
			log.Errorf(c, "Log stream is not archived.")
			return errors.New("log stream is not archived")
		}

		ls.Updated = now
		ls.State = coordinator.LSDone

		// Update the log stream.
		if err := ls.Put(di); err != nil {
			log.WithError(err).Errorf(c, "Failed to update log stream.")
			return err
		}

		log.Infof(c, "Successfully marked stream as cleaned up.")
		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to mark stream as cleaned up.")
		return nil, grpcutil.Internal
	}

	return &google.Empty{}, nil
}
