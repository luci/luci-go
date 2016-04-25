// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
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

// TerminateStream is an idempotent stream state terminate operation.
func (s *Server) TerminateStream(c context.Context, req *logdog.TerminateStreamRequest) (*google.Empty, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	log.Fields{
		"path":          req.Path,
		"terminalIndex": req.TerminalIndex,
	}.Infof(c, "Request to terminate log stream.")

	if req.TerminalIndex < 0 {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Negative terminal index.")
	}

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid path (%s): %s", req.Path, err)
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

	// Initialize our log stream. This cannot fail since we have already validated
	// req.Path.
	ls := coordinator.LogStreamFromPath(path)

	// Initialize our archival parameters.
	params := coordinator.ArchivalParams{
		RequestID:      info.Get(c).RequestID(),
		SettleDelay:    cfg.Coordinator.ArchiveSettleDelay.Duration(),
		CompletePeriod: cfg.Coordinator.ArchiveDelayMax.Duration(),
	}

	// Transactionally validate and update the terminal index.
	err = ds.Get(c).RunInTransaction(func(c context.Context) error {
		if err := ds.Get(c).Get(ls); err != nil {
			if err == ds.ErrNoSuchEntity {
				log.Debugf(c, "LogEntry not found.")
				return grpcutil.Errf(codes.NotFound, "Log stream %q is not registered", req.Path)
			}

			log.WithError(err).Errorf(c, "Failed to load LogEntry.")
			return grpcutil.Internal
		}

		switch {
		case subtle.ConstantTimeCompare(ls.Secret, req.Secret) != 1:
			log.Errorf(c, "Secrets do not match.")
			return grpcutil.Errf(codes.InvalidArgument, "Request secret doesn't match the stream secret.")

		case ls.State > coordinator.LSStreaming:
			// Succeed if this is non-conflicting (idempotent).
			if ls.TerminalIndex == req.TerminalIndex {
				log.Fields{
					"state":         ls.State.String(),
					"terminalIndex": ls.TerminalIndex,
				}.Infof(c, "Log stream is already terminated.")
				return nil
			}

			log.Fields{
				"state":         ls.State.String(),
				"terminalIndex": ls.TerminalIndex,
			}.Warningf(c, "Log stream is not in streaming state.")
			return grpcutil.Errf(codes.FailedPrecondition, "Log stream is not in streaming state.")

		default:
			// Everything looks good, let's proceed...
			ls.TerminalIndex = req.TerminalIndex
			ls.TerminatedTime = ds.RoundTime(clock.Now(c).UTC())

			// Create an archival task.
			if err := params.PublishTask(c, ap, ls); err != nil {
				if err == coordinator.ErrArchiveTasked {
					log.Warningf(c, "Archival has already been tasked for this stream.")
					return nil
				}

				log.WithError(err).Errorf(c, "Failed to create archive task.")
				return grpcutil.Internal
			}

			if err := ds.Get(c).Put(ls); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return grpcutil.Internal
			}

			log.Fields{
				"terminalIndex": ls.TerminalIndex,
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
