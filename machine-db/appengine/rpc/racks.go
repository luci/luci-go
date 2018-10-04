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
	"context"
	"database/sql"

	"github.com/Masterminds/squirrel"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// ListRacks handles a request to retrieve racks.
func (*Service) ListRacks(c context.Context, req *crimson.ListRacksRequest) (*crimson.ListRacksResponse, error) {
	racks, err := listRacks(c, req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListRacksResponse{
		Racks: racks,
	}, nil
}

// listRacks returns a slice of racks in the database.
func listRacks(c context.Context, req *crimson.ListRacksRequest) ([]*crimson.Rack, error) {
	stmt := squirrel.Select("r.name", "r.description", "r.state", "d.name", "h.name").
		From("(racks r, datacenters d)").
		LeftJoin("kvms k ON r.kvm_id = k.id").
		LeftJoin("hostnames h ON k.hostname_id = h.id").
		Where("r.datacenter_id = d.id")
	stmt = selectInString(stmt, "r.name", req.Names)
	stmt = selectInString(stmt, "d.name", req.Datacenters)
	stmt = selectInString(stmt, "h.name", req.Kvms)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	db := database.Get(c)
	rows, err := db.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch racks").Err()
	}
	defer rows.Close()

	var racks []*crimson.Rack
	for rows.Next() {
		rack := &crimson.Rack{}
		var kvm sql.NullString
		if err = rows.Scan(&rack.Name, &rack.Description, &rack.State, &rack.Datacenter, &kvm); err != nil {
			return nil, errors.Annotate(err, "failed to fetch rack").Err()
		}
		if kvm.Valid {
			rack.Kvm = kvm.String
		}
		racks = append(racks, rack)
	}
	return racks, nil
}
