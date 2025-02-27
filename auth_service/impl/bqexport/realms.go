// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bqexport

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
)

// parseRealms parses the realms from the given AuthDB to construct realm rows
// which can then be exported to BQ.
func parseRealms(ctx context.Context, authDB *protocol.AuthDB,
	authDBRev int64, ts *timestamppb.Timestamp) ([]*bqpb.RealmRow, error) {
	realms := authDB.Realms
	if realms == nil {
		return []*bqpb.RealmRow{}, customerrors.ErrAuthDBMissingRealms
	}
	sizeHint := len(realms.Realms)
	if sizeHint == 0 {
		// Log a warning because it is very unlikely the AuthDB has no realms.
		logging.Warningf(ctx, "no realms in AuthDB")
		return []*bqpb.RealmRow{}, nil
	}
	toPermissionNames := func(indices []uint32) []string {
		names := make([]string, len(indices))
		for i, permIndex := range indices {
			names[i] = realms.Permissions[permIndex].Name
		}
		return names
	}
	// Process each condition to the (attribute, value) pairs associated with it.
	conditions := make([][]string, len(realms.Conditions))
	for i, condition := range realms.Conditions {
		restriction := condition.GetRestrict()
		values := stringset.New(len(restriction.Values))
		for _, value := range restriction.Values {
			values.Add(fmt.Sprintf("%s==%s", restriction.Attribute, value))
		}
		conditions[i] = values.ToSortedSlice()
	}
	toConditionValues := func(indices []uint32) []string {
		// Set the initial capacity assuming every condition has at least one value.
		values := stringset.New(len(indices))
		for _, condIndex := range indices {
			values.AddAll(conditions[condIndex])
		}
		return values.ToSortedSlice()
	}
	// Set the initial capacity assuming every realm has at least one binding.
	realmRows := make([]*bqpb.RealmRow, 0, sizeHint)
	for _, realm := range realms.Realms {
		for i, binding := range realm.Bindings {
			realmRows = append(realmRows, &bqpb.RealmRow{
				Name:        realm.Name,
				BindingId:   int64(i),
				Permissions: toPermissionNames(binding.Permissions),
				Principals:  binding.Principals,
				Conditions:  toConditionValues(binding.Conditions),
				AuthdbRev:   authDBRev,
				ExportedAt:  ts,
			})
		}
	}
	return realmRows, nil
}
