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

// ListDatacenters handles a request to retrieve datacenters.
func (*Service) ListDatacenters(c context.Context, req *crimson.ListDatacentersRequest) (*crimson.ListDatacentersResponse, error) {
	datacenters, err := listDatacenters(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, err
	}
	return &crimson.ListDatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// listDatacenters returns a slice of datacenters in the database.
func listDatacenters(c context.Context, names stringset.Set) ([]*crimson.Datacenter, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT name, description, state
		FROM datacenters
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	var datacenters []*crimson.Datacenter
	for rows.Next() {
		dc := &crimson.Datacenter{}
		if err = rows.Scan(&dc.Name, &dc.Description, &dc.State); err != nil {
			return nil, errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if matches(dc.Name, names) {
			datacenters = append(datacenters, dc)
		}
	}
	return datacenters, nil
}
