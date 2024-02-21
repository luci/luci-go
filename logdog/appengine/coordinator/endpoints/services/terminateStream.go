// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/types"
)

var (
	terminateStreamMetric = metric.NewCounter(
		"logdog/endpoints/terminate_stream",
		"Requests to terminate stream",
		nil,
		field.String("project"))
)

// TerminateStream is an idempotent stream state terminate operation.
func (s *server) TerminateStream(c context.Context, req *logdog.TerminateStreamRequest) (*emptypb.Empty, error) {
	if req.TerminalIndex < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Negative terminal index.")
	}

	id := coordinator.HashID(req.Id)
	if err := id.Normalize(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid ID (%s): %s", id, err)
	}

	// Initialize our log stream state.
	lst := coordinator.NewLogStreamState(c, id)

	// Load the Stream and Prefix.
	ls := &coordinator.LogStream{ID: lst.ID()}
	if err := ds.Get(c, ls); err != nil {
		log.WithError(err).Errorf(c, "Failed to load log stream.")
		if err == ds.ErrNoSuchEntity {
			return nil, status.Errorf(codes.NotFound, "log stream doesn't exist")
		}
		return nil, status.Error(codes.Internal, "internal server error")
	}
	pfx := &coordinator.LogPrefix{ID: coordinator.LogPrefixID(types.StreamName(ls.Prefix))}
	if err := ds.Get(c, pfx); err != nil {
		log.WithError(err).Errorf(c, "Failed to load log stream prefix.")
		if err == ds.ErrNoSuchEntity {
			return nil, status.Errorf(codes.Internal, "prefix is not registered")
		}
		return nil, status.Error(codes.Internal, "internal server error")
	}

	// Transactionally validate and update the terminal index.
	err := ds.RunInTransaction(c, func(c context.Context) error {
		if err := ds.Get(c, lst); err != nil {
			if err == ds.ErrNoSuchEntity {
				log.Debugf(c, "Log stream state not found.")
				return status.Errorf(codes.NotFound, "Log stream %q is not registered", id)
			}

			log.WithError(err).Errorf(c, "Failed to load LogEntry.")
			return status.Error(codes.Internal, "internal server error")
		}

		switch {
		case subtle.ConstantTimeCompare(lst.Secret, req.Secret) != 1:
			log.Errorf(c, "Secrets do not match.")
			return status.Errorf(codes.InvalidArgument, "Request secret doesn't match the stream secret.")

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
			return status.Errorf(codes.FailedPrecondition, "Log stream is incompatibly terminated.")

		default:
			// Everything looks good, let's proceed...
			now := clock.Now(c).UTC()
			lst.Updated = now
			lst.TerminalIndex = req.TerminalIndex
			lst.TerminatedTime = now

			if err := ds.Put(c, lst); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return status.Error(codes.Internal, "internal server error")
			}
		}

		return s.taskArchival(c, lst, pfx.Realm, optimisticArchivalDelay)
	}, nil)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return nil, err
	}

	terminateStreamMetric.Add(c, 1, req.Project)
	return &emptypb.Empty{}, nil
}
