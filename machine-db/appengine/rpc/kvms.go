// Copyright 2018 The LUCI Authors.
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

	"github.com/Masterminds/squirrel"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// ListKVMs handles a request to retrieve KVMs.
func (*Service) ListKVMs(c context.Context, req *crimson.ListKVMsRequest) (*crimson.ListKVMsResponse, error) {
	kvms, err := listKVMs(c, req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListKVMsResponse{
		Kvms: kvms,
	}, nil
}

// listKVMs returns a slice of KVMs in the database.
func listKVMs(c context.Context, req *crimson.ListKVMsRequest) ([]*crimson.KVM, error) {
	ipv4s, err := parseIPv4s(req.Ipv4S)
	if err != nil {
		return nil, err
	}
	mac48s, err := parseMAC48s(req.MacAddresses)
	if err != nil {
		return nil, err
	}

	stmt := squirrel.Select(
		"h.name",
		"h.vlan_id",
		"p.name",
		"r.name",
		"d.name",
		"k.description",
		"k.mac_address",
		"i.ipv4",
		"k.state",
	)
	stmt = stmt.From("kvms k, hostnames h, platforms p, racks r, datacenters d, ips i").
		Where("k.hostname_id = h.id").
		Where("k.platform_id = p.id").
		Where("k.rack_id = r.id").
		Where("r.datacenter_id = d.id").
		Where("i.hostname_id = h.id")
	stmt = selectInInt64(stmt, "h.vlan_id", req.Vlans)
	stmt = selectInString(stmt, "h.name", req.Names)
	stmt = selectInString(stmt, "p.name", req.Platforms)
	stmt = selectInString(stmt, "d.name", req.Datacenters)
	stmt = selectInString(stmt, "r.name", req.Racks)
	stmt = selectInUint64(stmt, "k.mac_address", mac48s)
	stmt = selectInInt64(stmt, "i.ipv4", ipv4s)
	stmt = selectInState(stmt, "k.state", req.States)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	db := database.Get(c)
	rows, err := db.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch KVMs").Err()
	}
	defer rows.Close()

	var kvms []*crimson.KVM
	for rows.Next() {
		k := &crimson.KVM{}
		var ipv4 common.IPv4
		var mac48 common.MAC48
		if err = rows.Scan(
			&k.Name,
			&k.Vlan,
			&k.Platform,
			&k.Rack,
			&k.Datacenter,
			&k.Description,
			&mac48,
			&ipv4,
			&k.State,
		); err != nil {
			return nil, errors.Annotate(err, "failed to fetch KVM").Err()
		}
		k.MacAddress = mac48.String()
		k.Ipv4 = ipv4.String()
		kvms = append(kvms, k)
	}
	return kvms, nil
}
