// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// TerminateStream is an idempotent stream state terminate operation.
func (s *server) TerminateStream(c context.Context, req *logdog.TerminateStreamRequest) (*google.Empty, error) {
	log.Fields{
		"project":       req.Project,
		"id":            req.Id,
		"terminalIndex": req.TerminalIndex,
	}.Infof(c, "Request to terminate log stream.")

	if req.TerminalIndex < 0 {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Negative terminal index.")
	}

	id := coordinator.HashID(req.Id)
	if err := id.Normalize(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid ID (%s): %s", id, err)
	}

	svc := coordinator.GetServices(c)
	_, cfg, err := svc.Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load configuration.")
		return nil, grpcutil.Internal
	}

	ap, err := svc.ArchivalPublisher(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get archival publisher instance.")
		return nil, grpcutil.Internal
	}

	// Initialize our log stream state.
	di := ds.Get(c)
	lst := coordinator.NewLogStreamState(di, id)

	// Initialize our archival parameters.
	params := coordinator.ArchivalParams{
		RequestID:      info.Get(c).RequestID(),
		SettleDelay:    cfg.Coordinator.ArchiveSettleDelay.Duration(),
		CompletePeriod: cfg.Coordinator.ArchiveDelayMax.Duration(),
	}

	// Transactionally validate and update the terminal index.
	err = di.RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)

		if err := di.Get(lst); err != nil {
			if err == ds.ErrNoSuchEntity {
				log.Debugf(c, "Log stream state not found.")
				return grpcutil.Errf(codes.NotFound, "Log stream %q is not registered", id)
			}

			log.WithError(err).Errorf(c, "Failed to load LogEntry.")
			return grpcutil.Internal
		}

		switch {
		case subtle.ConstantTimeCompare(lst.Secret, req.Secret) != 1:
			log.Errorf(c, "Secrets do not match.")
			return grpcutil.Errf(codes.InvalidArgument, "Request secret doesn't match the stream secret.")

		case lst.Terminated():
			// Succeed if this is non-conflicting (idempotent).
			if lst.TerminalIndex == req.TerminalIndex {
				log.Fields{
					"terminalIndex": lst.TerminalIndex,
				}.Infof(c, "Log stream is already terminated.")
				return nil
			}

			log.Fields{
				"terminalIndex": lst.TerminalIndex,
			}.Warningf(c, "Log stream is already incompatibly terminated.")
			return grpcutil.Errf(codes.FailedPrecondition, "Log stream is incompatibly terminated.")

		default:
			// Everything looks good, let's proceed...
			now := clock.Now(c).UTC()
			lst.Updated = now
			lst.TerminalIndex = req.TerminalIndex
			lst.TerminatedTime = now

			// Create an archival task.
			if err := params.PublishTask(c, ap, lst); err != nil {
				if err == coordinator.ErrArchiveTasked {
					log.Warningf(c, "Archival has already been tasked for this stream.")
					return nil
				}

				log.WithError(err).Errorf(c, "Failed to create archive task.")
				return grpcutil.Internal
			}

			if err := di.Put(lst); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return grpcutil.Internal
			}

			// Delete the archive expiration mutation, since we have just dispatched
			// an archive request.
			aeParent, aeName := (&mutations.CreateArchiveTask{ID: id}).TaskName(di)
			if err := tumble.CancelNamedMutations(c, aeParent, aeName); err != nil {
				log.WithError(err).Errorf(c, "Failed to cancel archive expiration mutation.")
				return grpcutil.Internal
			}

			log.Fields{
				"terminalIndex": lst.TerminalIndex,
			}.Infof(c, "Terminal index was set and archival was dispatched.")
			return nil
		}
	}, nil)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return nil, err
	}

	return &google.Empty{}, nil
}
