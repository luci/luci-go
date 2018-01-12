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

// ListRacks handles a request to retrieve racks.
func (*Service) ListRacks(c context.Context, req *crimson.ListRacksRequest) (*crimson.ListRacksResponse, error) {
	racks, err := listRacks(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Datacenters...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListRacksResponse{
		Racks: racks,
	}, nil
}

// listRacks returns a slice of racks in the database.
func listRacks(c context.Context, names, datacenters stringset.Set) ([]*crimson.Rack, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT r.name, r.description, d.name
		FROM racks r, datacenters d
		WHERE r.datacenter_id = d.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch racks").Err()
	}
	defer rows.Close()

	var racks []*crimson.Rack
	for rows.Next() {
		rack := &crimson.Rack{}
		if err = rows.Scan(&rack.Name, &rack.Description, &rack.Datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch rack").Err()
		}
		if matches(rack.Name, names) && matches(rack.Datacenter, datacenters) {
			racks = append(racks, rack)
		}
	}
	return racks, nil
}
