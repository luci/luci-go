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
	"strings"

	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Masterminds/squirrel"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// CreateDRAC handles a request to create a new DRAC.
func (*Service) CreateDRAC(c context.Context, req *crimson.CreateDRACRequest) (*crimson.DRAC, error) {
	drac, err := createDRAC(c, req.Drac)
	if err != nil {
		return nil, err
	}
	return drac, nil
}

// ListDRACs handles a request to list DRACs.
func (*Service) ListDRACs(c context.Context, req *crimson.ListDRACsRequest) (*crimson.ListDRACsResponse, error) {
	dracs, err := listDRACs(c, database.Get(c), req)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListDRACsResponse{
		Dracs: dracs,
	}, nil
}

// createDRAC creates a new DRAC in the database.
func createDRAC(c context.Context, d *crimson.DRAC) (*crimson.DRAC, error) {
	if err := validateDRACForCreation(d); err != nil {
		return nil, err
	}
	ip, _ := common.ParseIPv4(d.Ipv4)
	tx, err := database.Begin(c)
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}
	defer tx.MaybeRollback(c)

	hostnameId, err := assignHostnameAndIP(c, tx, d.Name, ip)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(c, `
		INSERT INTO dracs (hostname_id, machine_id)
		VALUES (?, (SELECT id FROM machines WHERE name = ?))
	`, hostnameId, d.Machine)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate DRAC for machine %q", d.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'machine_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "unknown machine %q", d.Machine)
		}
		return nil, internalError(c, errors.Annotate(err, "failed to create DRAC").Err())
	}

	dracs, err := listDRACs(c, tx, &crimson.ListDRACsRequest{
		// DRACs are typically identified by hostname and VLAN, but VLAN is inferred from IP address during creation.
		Names: []string{d.Name},
		Ipv4S: []string{d.Ipv4},
	})
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to fetch created DRAC").Err())
	}

	if err := tx.Commit(); err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return dracs[0], nil
}

// listDRACs returns a slice of DRACs in the database.
func listDRACs(c context.Context, q database.QueryerContext, req *crimson.ListDRACsRequest) ([]*crimson.DRAC, error) {
	ipv4s, err := parseIPv4s(req.Ipv4S)
	if err != nil {
		return nil, err
	}

	stmt := squirrel.Select("h.name", "h.vlan_id", "m.name", "i.ipv4").
		From("dracs d, hostnames h, machines m, ips i").
		Where("d.hostname_id = h.id").
		Where("d.machine_id = m.id").
		Where("i.hostname_id = h.id")
	stmt = selectInString(stmt, "h.name", req.Names)
	stmt = selectInString(stmt, "m.name", req.Machines)
	stmt = selectInInt64(stmt, "i.ipv4", ipv4s)
	stmt = selectInInt64(stmt, "h.vlan_id", req.Vlans)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to generate statement").Err())
	}

	rows, err := q.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch DRACs").Err()
	}
	defer rows.Close()
	var dracs []*crimson.DRAC
	for rows.Next() {
		d := &crimson.DRAC{}
		var ipv4 common.IPv4
		if err = rows.Scan(&d.Name, &d.Vlan, &d.Machine, &ipv4); err != nil {
			return nil, errors.Annotate(err, "failed to fetch DRAC").Err()
		}
		d.Ipv4 = ipv4.String()
		dracs = append(dracs, d)
	}
	return dracs, nil
}

// validateDRACForCreation validates a DRAC for creation.
func validateDRACForCreation(d *crimson.DRAC) error {
	switch {
	case d == nil:
		return status.Error(codes.InvalidArgument, "DRAC specification is required")
	case d.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case d.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case d.Vlan != 0:
		return status.Error(codes.InvalidArgument, "VLAN must not be specified, use IP address instead")
	default:
		_, err := common.ParseIPv4(d.Ipv4)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid IPv4 address %q", d.Ipv4)
		}
		return nil
	}
}
