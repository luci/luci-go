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

// ListSwitches handles a request to retrieve switches.
func (*Service) ListSwitches(c context.Context, req *crimson.ListSwitchesRequest) (*crimson.ListSwitchesResponse, error) {
	switches, err := listSwitches(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Racks...), stringset.NewFromSlice(req.Datacenters...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListSwitchesResponse{
		Switches: switches,
	}, nil
}

// listSwitches returns a slice of switches in the database.
func listSwitches(c context.Context, names, racks, datacenters stringset.Set) ([]*crimson.Switch, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT s.name, s.description, s.ports, r.name, d.name
		FROM switches s, racks r, datacenters d
		WHERE s.rack_id = r.id
			AND r.datacenter_id = d.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch switches").Err()
	}
	defer rows.Close()

	var switches []*crimson.Switch
	for rows.Next() {
		s := &crimson.Switch{}
		if err = rows.Scan(&s.Name, &s.Description, &s.Ports, &s.Rack, &s.Datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch switch").Err()
		}
		if matches(s.Name, names) && matches(s.Rack, racks) && matches(s.Datacenter, datacenters) {
			switches = append(switches, s)
		}
	}
	return switches, nil
}
