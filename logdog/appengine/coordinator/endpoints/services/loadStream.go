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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	ds "go.chromium.org/luci/gae/service/datastore"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
)

// LoadStream loads the log stream state.
func (s *server) LoadStream(c context.Context, req *logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
	id := coordinator.HashID(req.Id)
	if err := id.Normalize(); err != nil {
		log.WithError(err).Errorf(c, "Invalid stream ID.")
		return nil, status.Errorf(codes.InvalidArgument, "Invalid ID (%s): %s", id, err)
	}

	ls := &coordinator.LogStream{ID: coordinator.HashID(req.Id)}
	lst := ls.State(c)

	if err := ds.Get(c, lst, ls); err != nil {
		if anyNoSuchEntity(err) {
			log.WithError(err).Errorf(c, "No such entity in datastore (log stream).")

			// The state isn't registered, so this stream does not exist.
			return nil, status.Errorf(codes.NotFound, "Log stream was not found.")
		}

		log.WithError(err).Errorf(c, "Failed to load log stream.")
		return nil, status.Error(codes.Internal, "internal server error")
	}

	// The log stream and state loaded successfully.
	resp := logdog.LoadStreamResponse{
		State: buildLogStreamState(ls, lst),
	}
	if req.Desc {
		resp.Desc = ls.Descriptor
	}
	resp.ArchivalKey = lst.ArchivalKey
	resp.Age = durationpb.New(ds.RoundTime(clock.Now(c)).Sub(lst.Updated))

	return &resp, nil
}

func anyNoSuchEntity(err error) bool {
	return errors.Any(err, func(err error) bool {
		return err == ds.ErrNoSuchEntity
	})
}
