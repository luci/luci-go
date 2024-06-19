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

package model

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/service/protocol"
)

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

// Snapshot contains transactionally captured AuthDB entities.
type Snapshot struct {
	ReplicationState *AuthReplicationState
	GlobalConfig     *AuthGlobalConfig
	Groups           []*AuthGroup
	IPAllowlists     []*AuthIPAllowlist
	RealmsGlobals    *AuthRealmsGlobals
	ProjectRealms    []*AuthProjectRealms

	// TODO:
	//   IPAllowlistAssignments
}

// TakeSnapshot takes a consistent snapshot of the replicated subset of AuthDB
// entities.
//
// Runs a read-only transaction internally.
func TakeSnapshot(ctx context.Context) (snap *Snapshot, err error) {
	// This is a potentially slow operation. Capture it in the trace.
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/auth_service/impl/model/TakeSnapshot")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		snap = &Snapshot{
			ReplicationState: &AuthReplicationState{
				Kind:   "AuthReplicationState",
				ID:     "self",
				Parent: RootKey(ctx),
			},
			GlobalConfig: &AuthGlobalConfig{
				Kind: "AuthGlobalConfig",
				ID:   "root",
			},
		}

		gr, ctx := errgroup.WithContext(ctx)
		gr.Go(func() error {
			return datastore.Get(ctx, snap.GlobalConfig)
		})
		gr.Go(func() error {
			return datastore.Get(ctx, snap.ReplicationState)
		})
		gr.Go(func() (err error) {
			snap.Groups, err = GetAllAuthGroups(ctx)
			return
		})
		gr.Go(func() (err error) {
			snap.IPAllowlists, err = GetAllAuthIPAllowlists(ctx)
			return
		})
		gr.Go(func() (err error) {
			snap.RealmsGlobals, err = GetAuthRealmsGlobals(ctx)
			return
		})
		gr.Go(func() (err error) {
			snap.ProjectRealms, err = GetAllAuthProjectRealms(ctx)
			return
		})

		// TODO:
		//  IPAllowlistAssignments

		return gr.Wait()
	}, &datastore.TransactionOptions{ReadOnly: true})

	if err != nil {
		return nil, err
	}
	return snap, nil
}

// ToAuthDBProto converts the snapshot to an AuthDB proto message.
func (s *Snapshot) ToAuthDBProto(ctx context.Context, useV1Perms bool) (*protocol.AuthDB, error) {
	groups := make([]*protocol.AuthGroup, len(s.Groups))
	for i, v := range s.Groups {
		groups[i] = &protocol.AuthGroup{
			Name:        v.ID,
			Members:     v.Members,
			Globs:       v.Globs,
			Nested:      v.Nested,
			Description: setStringField(v.Description),
			CreatedTs:   v.CreatedTS.UnixNano() / 1000,
			CreatedBy:   v.CreatedBy,
			ModifiedTs:  v.ModifiedTS.UnixNano() / 1000,
			ModifiedBy:  v.ModifiedBy,
			Owners:      v.Owners,
		}
	}

	allowlists := make([]*protocol.AuthIPWhitelist, len(s.IPAllowlists))
	for i, v := range s.IPAllowlists {
		allowlists[i] = &protocol.AuthIPWhitelist{
			Name:        v.ID,
			Subnets:     v.Subnets,
			Description: setStringField(v.Description),
			CreatedTs:   v.CreatedTS.UnixNano() / 1000,
			CreatedBy:   v.CreatedBy,
			ModifiedTs:  v.ModifiedTS.UnixNano() / 1000,
			ModifiedBy:  v.ModifiedBy,
		}
	}

	realms, err := MergeRealms(ctx, s.RealmsGlobals, s.ProjectRealms, useV1Perms)
	if err != nil {
		return nil, errors.Annotate(err, "error merging realms").Err()
	}

	return &protocol.AuthDB{
		OauthClientId:            setStringField(s.GlobalConfig.OAuthClientID),
		OauthClientSecret:        setStringField(s.GlobalConfig.OAuthClientSecret),
		OauthAdditionalClientIds: s.GlobalConfig.OAuthAdditionalClientIDs,
		TokenServerUrl:           setStringField(s.GlobalConfig.TokenServerURL),
		SecurityConfig:           s.GlobalConfig.SecurityConfig,
		Groups:                   groups,
		IpWhitelists:             allowlists,
		Realms:                   realms,
		IpWhitelistAssignments:   nil, // TODO
	}, nil
}

// setStringField returns a non-empty string, given a string; it
// defaults to the string value "empty".
func setStringField(value string) string {
	if value != "" {
		return value
	}
	return "empty"
}

// ToAuthDB converts the snapshot to an authdb.SnapshotDB.
//
// It then can be used by the auth service itself to make ACL checks.
func (s *Snapshot) ToAuthDB(ctx context.Context, useV1Perms bool) (*authdb.SnapshotDB, error) {
	authDBProto, err := s.ToAuthDBProto(ctx, useV1Perms)
	if err != nil {
		return nil, errors.Annotate(err,
			"failed converting AuthDB snapshot to proto").Err()
	}
	return authdb.NewSnapshotDB(authDBProto, "", s.ReplicationState.AuthDBRev, false)
}
