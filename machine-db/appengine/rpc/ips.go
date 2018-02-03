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
	"database/sql"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// ListIPs handles a request to list IP addresses.
func (*Service) ListIPs(c context.Context, req *crimson.ListIPsRequest) (*crimson.ListIPsResponse, error) {
	vlans := make(map[int64]struct{}, len(req.Vlans))
	for _, vlan := range req.Vlans {
		vlans[vlan] = struct{}{}
	}
	ips, err := listIPs(c, stringset.NewFromSlice(req.Ipv4S...), vlans)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListIPsResponse{
		Ips: ips,
	}, nil
}

// listIPs returns a slice of IP addresses in the database.
func listIPs(c context.Context, ipv4s stringset.Set, vlans map[int64]struct{}) ([]*crimson.IP, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT i.ipv4, i.vlan_id, h.name
		FROM ips i
		LEFT OUTER JOIN hostnames h
			ON i.hostname_id = h.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch IP addresses").Err()
	}
	defer rows.Close()

	var ips []*crimson.IP
	for rows.Next() {
		ip := &crimson.IP{}
		var ipv4 common.IPv4
		var hostname sql.NullString
		if err = rows.Scan(&ipv4, &ip.Vlan, &hostname); err != nil {
			return nil, errors.Annotate(err, "failed to fetch IP address").Err()
		}
		if _, ok := vlans[ip.Vlan]; matches(ipv4.String(), ipv4s) && (ok || len(vlans) == 0) {
			ip.Ipv4 = ipv4.String()
			if hostname.Valid {
				ip.Hostname = hostname.String
			}
			ips = append(ips, ip)
		}
	}
	return ips, nil
}
