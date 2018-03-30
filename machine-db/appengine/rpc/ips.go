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
		return nil, err
	}
	return &crimson.ListIPsResponse{
		Ips: ips,
	}, nil
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

// parseIPv4s returns a slice of int64 IP addresses.
func parseIPv4s(ips []string) ([]int64, error) {
	ipv4s := make([]int64, len(ips))
	for i, ip := range ips {
		ipv4, err := common.ParseIPv4(ip)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid IPv4 address %q", ip)
		}
		ipv4s[i] = int64(ipv4)
	}
	return ipv4s, nil
}
