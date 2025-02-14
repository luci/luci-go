// Copyright 2025 The LUCI Authors.
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

// Package model contains model definitions for Auth Service.
//
// This file contains functionality related to getting permissions
// from the latest Realms object.
package model

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/util/zlib"
)

var (
	ErrSnapshotMissingAuthDB = errors.New("AuthDBSnapshot missing AuthDB field")
)

// processSnapshot is a helper function to get the
// ReplicationPushRequest from the given AuthDBSnapshot.
func processSnapshot(authDBSnapshot *AuthDBSnapshot) (*protocol.ReplicationPushRequest, error) {
	authDBBlob, err := zlib.Decompress(authDBSnapshot.AuthDBDeflated)
	if err != nil {
		return nil, errors.Annotate(err, "error decompressing AuthDBDeflated").Err()
	}

	req := &protocol.ReplicationPushRequest{}
	if err := proto.Unmarshal(authDBBlob, req); err != nil {
		return nil, errors.Annotate(err, "error unmarshalling AuthDB blob").Err()
	}

	return req, nil
}

// GetAuthDBFromSnapshot returns the AuthDB at the given revision, if an
// AuthDBSnapshot at that revision exists.
func GetAuthDBFromSnapshot(ctx context.Context, authDBRev int64) (*protocol.AuthDB, error) {
	snapshot, err := GetAuthDBSnapshot(ctx, authDBRev, false)
	if err != nil {
		return nil, err
	}
	// Decompress and unmarshal snapshot.
	req, err := processSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	if req.AuthDb == nil {
		return nil, ErrSnapshotMissingAuthDB
	}

	return req.AuthDb, nil
}

// AnalyzePrincipalPermissions analyzes the realms for the permissions granted
// to each principal in a given realm.
//
// It returns a mapping of principal -> collection of realm permissions
// (the permissions that principal has in that specific realm).
func AnalyzePrincipalPermissions(realms *protocol.Realms) (map[string][]*rpcpb.RealmPermissions, error) {
	result := make(map[string][]*rpcpb.RealmPermissions)
	for _, realm := range realms.Realms {
		principalToPermissions := make(map[string]stringset.Set)
		// Maintain map of principal to permissions set.
		for _, binding := range realm.Bindings {
			permissionNames := stringset.New(len(binding.Permissions))
			for _, permIndex := range binding.Permissions {
				permissionNames.Add(realms.Permissions[permIndex].Name)
			}
			// For each principal look up its current permission set & extend it by these permission names.
			permissions := permissionNames.ToSlice()
			for _, principal := range binding.Principals {
				if _, ok := principalToPermissions[principal]; !ok {
					principalToPermissions[principal] = stringset.New(0)
				}
				principalToPermissions[principal].AddAll(permissions)
			}
		}
		for principal, permissions := range principalToPermissions {
			realmPermissions := &rpcpb.RealmPermissions{
				Name:        realm.Name,
				Permissions: permissions.ToSortedSlice(),
			}
			result[principal] = append(result[principal], realmPermissions)
		}
	}
	return result, nil
}
