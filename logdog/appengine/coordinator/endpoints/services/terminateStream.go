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

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
)

// TerminateStream is an idempotent stream state terminate operation.
func (s *server) TerminateStream(c context.Context, req *logdog.TerminateStreamRequest) (*empty.Empty, error) {
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

	// Initialize our log stream state.
	lst := coordinator.NewLogStreamState(c, id)

	// Transactionally validate and update the terminal index.
	err := ds.RunInTransaction(c, func(c context.Context) error {
		if err := ds.Get(c, lst); err != nil {
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

			if err := ds.Put(c, lst); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return grpcutil.Internal
			}
		}

		set := coordinator.GetSettings(c)
		return TaskArchival(c, lst, set.OptimisticArchivalDelay, set.OptimisticalArchivalPercent)
	}, nil)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return nil, err
	}

	return &empty.Empty{}, nil
}

func standardArchivalParams(cfg *config.Config, pcfg *svcconfig.ProjectConfig) *coordinator.ArchivalParams {
	return &coordinator.ArchivalParams{
		SettleDelay:    google.DurationFromProto(cfg.Coordinator.ArchiveSettleDelay),
		CompletePeriod: endpoints.MinDuration(cfg.Coordinator.ArchiveDelayMax, pcfg.MaxStreamAge),
	}
}
