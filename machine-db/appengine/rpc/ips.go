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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// defaultIPsPageSize is the default maximum number of IP addresses to return.
const defaultIPsPageSize = 10

// ListFreeIPs handles a request to list free IP addresses.
func (*Service) ListFreeIPs(c context.Context, req *crimson.ListFreeIPsRequest) (*crimson.ListIPsResponse, error) {
	switch {
	case req.Vlan < 1:
		return nil, status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	case req.PageSize < 0:
		return nil, status.Error(codes.InvalidArgument, "page size must be non-negative")
	case req.PageSize == 0:
		req.PageSize = defaultIPsPageSize
	}
	ips, err := listFreeIPs(c, req.Vlan, req.PageSize)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListIPsResponse{
		Ips: ips,
	}, nil
}

// getIP returns an IP address in the database.
func getIP(c context.Context, q database.QueryerContext, ipv4 common.IPv4) (*crimson.IP, error) {
	rows, err := q.QueryContext(c, `
		SELECT i.ipv4, i.vlan_id, h.name
		FROM ips i
		LEFT OUTER JOIN hostnames h
			ON i.hostname_id = h.id
		WHERE i.ipv4 = ?
	`, ipv4)
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to fetch IP address").Err())
	}
	defer rows.Close()

	// We only expect one result because IPv4 is unique in the database.
	if !rows.Next() {
		return nil, status.Errorf(codes.NotFound, "unknown IPv4 address %q", ipv4)
	}
	ip := &crimson.IP{}
	var hostname sql.NullString
	if err = rows.Scan(&ipv4, &ip.Vlan, &hostname); err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to fetch IP address").Err())
	}
	if hostname.Valid {
		ip.Hostname = hostname.String
	}
	ip.Ipv4 = ipv4.String()
	if rows.Next() {
		return nil, internalError(c, errors.Annotate(err, "unexpected number of rows").Err())
	}
	return ip, nil
}

// listFreeIPs returns a slice of free IP addresses in the database.
func listFreeIPs(c context.Context, vlan int64, limit int32) ([]*crimson.IP, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT ipv4
		FROM ips
		WHERE vlan_id = ? AND hostname_id IS NULL
		LIMIT ?
	`, vlan, limit)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch IP addresses").Err()
	}
	defer rows.Close()

	var ips []*crimson.IP
	for rows.Next() {
		ip := &crimson.IP{}
		var ipv4 common.IPv4
		if err = rows.Scan(&ipv4); err != nil {
			return nil, errors.Annotate(err, "failed to fetch IP address").Err()
		}
		ip.Ipv4 = ipv4.String()
		ip.Vlan = vlan
		ips = append(ips, ip)
	}
	return ips, nil
}
