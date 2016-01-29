// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var errAlreadyUpdated = errors.New("already updated")

// TerminateStream is an idempotent stream state terminate operation.
func (b *Server) TerminateStream(c context.Context, req *services.TerminateStreamRequest) (*google.Empty, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid path (%s): %s", req.Path, err)
	}
	c = log.SetField(c, "path", req.Path)

	if req.TerminalIndex < 0 {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Negative terminal index.")
	}

	// Initialize our log stream. This cannot fail since we have already validated
	// req.Path.
	ls := coordinator.LogStreamFromPath(path)
	switch err := updateTerminalIndex(c, ls, req); err {
	case errAlreadyUpdated:
		return &google.Empty{}, nil

	// To be confirmed/resolved transactionally.
	case nil:
		break
	default:
		// Because we're not in a transaction, forgive a "not found" status.
		if grpc.Code(err) != codes.NotFound {
			log.WithError(err).Errorf(c, "Failed to check LogStream status.")
			return nil, err
		}
	}

	// Transactionally update.
	now := clock.Now(c).UTC()
	err := ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)

		// Load the log stream state.
		switch err := updateTerminalIndex(c, ls, req); err {
		case nil:
			ls.Updated = now
			ls.State = coordinator.LSTerminated

			if err := ls.Put(di); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return grpcutil.Internal
			}

			log.Fields{
				"terminalIndex": ls.TerminalIndex,
			}.Infof(c, "Terminal index was set.")
			return nil

		case errAlreadyUpdated:
			return nil

		default:
			return err
		}
	}, &ds.TransactionOptions{XG: true})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return nil, err
	}

	return &google.Empty{}, nil
}

func updateTerminalIndex(c context.Context, ls *coordinator.LogStream, req *services.TerminateStreamRequest) error {
	if err := ds.Get(c).Get(ls); err != nil {
		if err == ds.ErrNoSuchEntity {
			log.Debugf(c, "LogEntry not found.")
			return grpcutil.Errf(codes.NotFound, "Log stream [%s] is not registered", req.Path)
		}

		log.WithError(err).Errorf(c, "Failed to load LogEntry.")
		return grpcutil.Internal
	}

	if subtle.ConstantTimeCompare(ls.Secret, req.Secret) != 1 {
		log.Errorf(c, "Secrets do not match.")
		return grpcutil.Errf(codes.InvalidArgument, "Request secret doesn't match the stream secret.")
	}

	switch {
	case ls.TerminalIndex == req.TerminalIndex:
		// Idempotent: already updated to this value.
		log.Debugf(c, "Log stream is already updated (idempotent).")
		return errAlreadyUpdated

	case ls.Terminated():
		// Terminated, but with a different value.
		log.Fields{
			"current":   ls.TerminalIndex,
			"requested": req.TerminalIndex,
		}.Warningf(c, "Refusing to change terminal index.")
		return grpcutil.Errf(codes.AlreadyExists, "Terminal index is already set.")

	default:
		ls.TerminalIndex = req.TerminalIndex
		return nil
	}
}
