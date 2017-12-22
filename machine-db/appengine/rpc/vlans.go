// Copyright 2017 The LUCI Authors.
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

package rpc

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// GetVLANs handles a request to retrieve vlans.
func (*Service) GetVLANs(c context.Context, req *crimson.VLANsRequest) (*crimson.VLANsResponse, error) {
	ids := make(map[int64]struct{}, len(req.Ids))
	for _, id := range req.Ids {
		ids[id] = struct{}{}
	}
	vlans, err := getVLANs(c, ids, stringset.NewFromSlice(req.Aliases...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.VLANsResponse{
		Vlans: vlans,
	}, nil
}

// getVLANs returns a slice of vlans in the database.
// Vlans matching either a given id or a given alias are returned. Specify no ids or aliases to return all vlans.
func getVLANs(c context.Context, ids map[int64]struct{}, aliases stringset.Set) ([]*crimson.VLAN, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT v.id, v.alias
		FROM vlans v
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch vlans").Err()
	}
	defer rows.Close()

	var vlans []*crimson.VLAN
	for rows.Next() {
		vlan := &crimson.VLAN{}
		if err = rows.Scan(&vlan.Id, &vlan.Alias); err != nil {
			return nil, errors.Annotate(err, "failed to fetch vlan").Err()
		}
		// Vlan may match either the given ids or aliases.
		// If both ids and aliases are empty, consider all vlans to match.
		if _, ok := ids[vlan.Id]; ok || aliases.Has(vlan.Alias) || (len(ids) == 0 && aliases.Len() == 0) {
			vlans = append(vlans, vlan)
		}
	}
	return vlans, nil
}
