// Copyright 2026 The LUCI Authors.
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

package logs

import (
	"context"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	ds "go.chromium.org/luci/gae/service/datastore"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/types"
)

func (s *server) DeletePrefix(ctx context.Context, req *logdog.DeletePrefixRequest) (*logdog.DeletePrefixResponse, error) {
	log.Fields{
		"project": req.Project,
		"prefix":  req.Prefix,
	}.Debugf(ctx, "Received DeletePrefix request.")

	prefix := types.StreamName(req.Prefix)
	if err := prefix.Validate(); err != nil {
		return nil, errors.Fmt(".prefix: %w", err)
	}

	var prefixEnt *coordinator.LogPrefix
	hid := coordinator.HashID(req.Prefix)
	if hid.Normalize() == nil {
		prefixEnt = &coordinator.LogPrefix{ID: hid}
	} else {
		prefixEnt = &coordinator.LogPrefix{ID: coordinator.LogPrefixID(types.StreamName(req.Prefix))}
	}

	now := clock.Now(ctx)

	err := ds.RunInTransaction(ctx, func(ctx context.Context) error {
		// 1) load the prefix
		if err := ds.Get(ctx, prefixEnt); err != nil {
			return err
		}

		// 2) check caller acl on prefix realm
		err := coordinator.CheckPermission(
			ctx,
			coordinator.PermDeletePrefix,
			types.StreamName(prefixEnt.Prefix),
			prefixEnt.Realm,
		)
		if err != nil {
			return err
		}

		// 3) update prefix to indicate that it has Expiration as <= now.
		// This will prevent any new streams from being added.
		if prefixEnt.Expiration.After(now) {
			prefixEnt.Expiration = now
			err = ds.Put(ctx, prefixEnt)
		}
		return nil
	}, nil)
	if err != nil {
		if errors.Is(err, ds.ErrNoSuchEntity) {
			return nil, status.Errorf(codes.NotFound, "prefix not found")
		}
		return nil, err
	}

	var eg errgroup.Group
	eg.SetLimit(16)
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		// Avoid leaking goroutines, but also don't block on any outstanding
		// deletes.
		cancel()
		eg.Wait()
	}()

	const batchSize = 50
	var streams []*coordinator.LogStream
	var states []*coordinator.LogStreamState
	flushBatch := func() {
		if len(streams) == 0 {
			return
		}
		myStreams := streams
		myStates := states
		// NOTE: Go will block if the errgroup is currently fully busy.
		eg.Go(func() error {
			// Delete the LogStream and the LogStreamState, ignoring any missing
			// entities.
			err := ds.Delete(ctx, myStreams, myStates)
			return errors.Filter(err, ds.ErrNoSuchEntity)
		})
		streams = nil
		states = nil
	}
	addEntry := func(ls *coordinator.LogStream) {
		streams = append(streams, ls)
		states = append(states, ls.State(ctx))
		if len(streams) >= batchSize {
			flushBatch()
		}
	}

	// 4) Scan all LogStreams+State for this prefix and delete them in batches.
	lsq, err := coordinator.NewLogStreamQuery(prefixEnt.Prefix)
	if err != nil {
		return nil, err
	}
	err = lsq.Run(ctx, func(ls *coordinator.LogStream, cc ds.CursorCB) error {
		addEntry(ls)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// If there is a partial batch, flush them before waiting for the group.
	flushBatch()

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// 5) Delete the prefix
	err = ds.Delete(ctx, prefixEnt)
	return &logdog.DeletePrefixResponse{}, err
}
