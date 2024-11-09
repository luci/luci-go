// Copyright 2021 The LUCI Authors.
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

package coordinator

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/logdog/common/types"
)

// MetadataFetcher fetches LogStream and LogPrefix metadata and checks ACLs.
type MetadataFetcher struct {
	Path      types.StreamPath // the log stream to fetch
	WithState bool             // if true, fetch LogStreamState as well

	Prefix *LogPrefix      // the fetched prefix
	Stream *LogStream      // the fetched stream
	State  *LogStreamState // the fetched state if WithState was true
}

// FetchWithACLCheck fetches the log stream entities and checks ACLs.
//
// Must be called within some project namespace. Returns gRPC errors, logs them
// inside.
func (f *MetadataFetcher) FetchWithACLCheck(ctx context.Context) error {
	if err := f.Path.Validate(); err != nil {
		logging.WithError(err).Errorf(ctx, "Invalid path supplied.")
		return status.Error(codes.InvalidArgument, "invalid path value")
	}
	prefix, _ := f.Path.Split()

	f.Prefix = &LogPrefix{ID: LogPrefixID(prefix)}
	f.Stream = &LogStream{ID: LogStreamID(f.Path)}
	if f.WithState {
		f.State = f.Stream.State(ctx)
	}

	logging.Fields{"id": f.Stream.ID}.Debugf(ctx, "Loading stream.")

	var ents []any
	if f.WithState {
		ents = []any{f.Prefix, f.Stream, f.State}
	} else {
		ents = []any{f.Prefix, f.Stream}
	}

	var prefixErr, streamErr, stateErr error
	if err := ds.Get(ctx, ents...); err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			if len(merr) != len(ents) {
				panic("impossible")
			}
			prefixErr, streamErr = merr[0], merr[1]
			if f.WithState {
				stateErr = merr[2]
			}
		} else {
			prefixErr, streamErr, stateErr = err, err, err
		}
	}

	// Treat ErrNoSuchEntity from the prefix in a special way: a non-existing
	// prefix should be handled in the same way as a prefix "hidden" by ACLs.
	switch {
	case prefixErr == ds.ErrNoSuchEntity:
		return PermissionDeniedErr(ctx)
	case prefixErr != nil:
		logging.WithError(prefixErr).Errorf(ctx, "Failed to fetch LogPrefix")
		return status.Error(codes.Internal, "internal server error")
	}

	// Old prefixes have no realm set. Fallback to "@legacy".
	realm := f.Prefix.Realm
	if realm == "" {
		realm = realms.Join(Project(ctx), realms.LegacyRealm)
	}

	// Check the caller is allowed to read streams under this prefix.
	if err := CheckPermission(ctx, PermLogsGet, prefix, realm); err != nil {
		return err
	}

	// Check if the stream actually exists. It is fine to expose this information
	// now after ACLs have been checked.
	switch {
	case streamErr == ds.ErrNoSuchEntity || stateErr == ds.ErrNoSuchEntity:
		logging.Warningf(ctx, "Log stream does not exist.")
		return status.Error(codes.NotFound, "stream path not found")
	case streamErr != nil:
		logging.WithError(streamErr).Errorf(ctx, "Failed to fetch LogStream")
		return status.Error(codes.Internal, "internal server error")
	case stateErr != nil:
		logging.WithError(stateErr).Errorf(ctx, "Failed to fetch LogStream")
		return status.Error(codes.Internal, "internal server error")
	}

	return nil
}
