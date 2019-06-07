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
	"strings"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Masterminds/squirrel"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/appengine/model"
	"go.chromium.org/luci/machine-db/common"
)

// CreateDRAC handles a request to create a new DRAC.
func (*Service) CreateDRAC(c context.Context, req *crimson.CreateDRACRequest) (*crimson.DRAC, error) {
	return createDRAC(c, req.Drac)
}

// ListDRACs handles a request to list DRACs.
func (*Service) ListDRACs(c context.Context, req *crimson.ListDRACsRequest) (*crimson.ListDRACsResponse, error) {
	dracs, err := listDRACs(c, database.Get(c), req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListDRACsResponse{
		Dracs: dracs,
	}, nil
}

// UpdateDRAC handles a request to update an existing DRAC.
func (*Service) UpdateDRAC(c context.Context, req *crimson.UpdateDRACRequest) (*crimson.DRAC, error) {
	return updateDRAC(c, req.Drac, req.UpdateMask)
}

// createDRAC creates a new DRAC in the database.
func createDRAC(c context.Context, d *crimson.DRAC) (*crimson.DRAC, error) {
	if err := validateDRACForCreation(d); err != nil {
		return nil, err
	}
	ip, _ := common.ParseIPv4(d.Ipv4)
	mac, _ := common.ParseMAC48(d.MacAddress)
	tx, err := database.Begin(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	hostnameID, err := model.AssignHostnameAndIP(c, tx, d.Name, ip)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(c, `
		INSERT INTO dracs (hostname_id, machine_id, switch_id, switchport, mac_address)
		VALUES (?, (SELECT id FROM machines WHERE name = ?), (SELECT id FROM switches WHERE name = ?), ?, ?)
	`, hostnameID, d.Machine, d.Switch, d.Switchport, mac)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate DRAC for machine %q", d.Machine)
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'mac_address'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'mac_address'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate MAC address %q", d.MacAddress)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'machine_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "machine %q does not exist", d.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'switch_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "switch %q does not exist", d.Switch)
		}
		return nil, errors.Annotate(err, "failed to create DRAC").Err()
	}

	dracs, err := listDRACs(c, tx, &crimson.ListDRACsRequest{
		Names: []string{d.Name},
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch created DRAC").Err()
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return dracs[0], nil
}

// listDRACs returns a slice of DRACs in the database.
func listDRACs(c context.Context, q database.QueryerContext, req *crimson.ListDRACsRequest) ([]*crimson.DRAC, error) {
	ipv4s, err := parseIPv4s(req.Ipv4S)
	if err != nil {
		return nil, err
	}
	mac48s, err := parseMAC48s(req.MacAddresses)
	if err != nil {
		return nil, err
	}

	stmt := squirrel.Select("h.name", "h.vlan_id", "m.name", "s.name", "d.switchport", "d.mac_address", "i.ipv4").
		From("dracs d, hostnames h, machines m, switches s, ips i").
		Where("d.hostname_id = h.id").
		Where("d.machine_id = m.id").
		Where("d.switch_id = s.id").
		Where("i.hostname_id = h.id")
	stmt = selectInString(stmt, "h.name", req.Names)
	stmt = selectInString(stmt, "m.name", req.Machines)
	stmt = selectInInt64(stmt, "i.ipv4", ipv4s)
	stmt = selectInInt64(stmt, "h.vlan_id", req.Vlans)
	stmt = selectInString(stmt, "s.name", req.Switches)
	stmt = selectInUint64(stmt, "d.mac_address", mac48s)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
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
		var mac48 common.MAC48
		if err = rows.Scan(&d.Name, &d.Vlan, &d.Machine, &d.Switch, &d.Switchport, &mac48, &ipv4); err != nil {
			return nil, errors.Annotate(err, "failed to fetch DRAC").Err()
		}
		d.Ipv4 = ipv4.String()
		d.MacAddress = mac48.String()
		dracs = append(dracs, d)
	}
	return dracs, nil
}

// updateDRAC updates an existing DRAC in the database.
func updateDRAC(c context.Context, d *crimson.DRAC, mask *field_mask.FieldMask) (*crimson.DRAC, error) {
	if err := validateDRACForUpdate(d, mask); err != nil {
		return nil, err
	}
	stmt := squirrel.Update("dracs")
	for _, path := range mask.Paths {
		switch path {
		case "machine":
			stmt = stmt.Set("machine_id", squirrel.Expr("(SELECT id FROM machines WHERE name = ?)", d.Machine))
		case "mac_address":
			mac, _ := common.ParseMAC48(d.MacAddress)
			stmt = stmt.Set("mac_address", mac)
		case "switch":
			stmt = stmt.Set("switch_id", squirrel.Expr("(SELECT id FROM switches WHERE name = ?)", d.Switch))
		case "switchport":
			stmt = stmt.Set("switchport", d.Switchport)
		}
	}
	stmt = stmt.Where("hostname_id = (SELECT id FROM hostnames WHERE name = ?)", d.Name)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	tx, err := database.Begin(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	_, err = tx.ExecContext(c, query, args...)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate DRAC for machine %q", d.Machine)
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'mac_address'"):
			// e.g. "Error 1062: Duplicate entry '1234567890' for key 'mac_address'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate MAC address %q", d.MacAddress)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "machine %q does not exist", d.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'switch_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "switch %q does not exist", d.Switch)
		}
		return nil, errors.Annotate(err, "failed to update DRAC").Err()
	}
	// The number of rows affected cannot distinguish between zero because the NIC didn't exist
	// and zero because the row already matched, so skip looking at the number of rows affected.

	dracs, err := listDRACs(c, tx, &crimson.ListDRACsRequest{
		Names: []string{d.Name},
	})
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch updated DRAC").Err()
	case len(dracs) == 0:
		return nil, status.Errorf(codes.NotFound, "DRAC %q does not exist", d.Name)
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return dracs[0], nil
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
	case d.Switch == "":
		return status.Error(codes.InvalidArgument, "switch is required and must be non-empty")
	case d.Switchport < 1:
		return status.Error(codes.InvalidArgument, "switchport must be positive")
	case d.Vlan != 0:
		return status.Error(codes.InvalidArgument, "VLAN must not be specified, use IP address instead")
	default:
		_, err := common.ParseIPv4(d.Ipv4)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid IPv4 address %q", d.Ipv4)
		}
		_, err = common.ParseMAC48(d.MacAddress)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", d.MacAddress)
		}
		return nil
	}
}

// validateDRACForUpdate validates a DRAC for update.
func validateDRACForUpdate(d *crimson.DRAC, mask *field_mask.FieldMask) error {
	switch err := validateUpdateMask(mask); {
	case d == nil:
		return status.Error(codes.InvalidArgument, "DRAC specification is required")
	case d.Name == "":
		return status.Error(codes.InvalidArgument, "DRAC name is required and must be non-empty")
	case err != nil:
		return err
	}
	for _, path := range mask.Paths {
		// TODO(smut): Allow IPv4 address to be updated.
		switch path {
		case "name":
			return status.Error(codes.InvalidArgument, "DRAC name cannot be updated, delete and create a new DRAC instead")
		case "vlan":
			return status.Error(codes.InvalidArgument, "VLAN cannot be updated, delete and create a new DRAC instead")
		case "machine":
			if d.Machine == "" {
				return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
			}
		case "mac_address":
			if d.MacAddress == "" {
				return status.Error(codes.InvalidArgument, "MAC address is required and must be non-empty")
			}
			_, err := common.ParseMAC48(d.MacAddress)
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", d.MacAddress)
			}
		case "switch":
			if d.Switch == "" {
				return status.Error(codes.InvalidArgument, "switch is required and must be non-empty")
			}
		case "switchport":
			if d.Switchport < 1 {
				return status.Error(codes.InvalidArgument, "switchport must be positive")
			}
		default:
			return status.Errorf(codes.InvalidArgument, "unsupported update mask path %q", path)
		}
	}
	return nil
}
