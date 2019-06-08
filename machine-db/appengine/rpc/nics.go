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
	"database/sql"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"

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

// CreateNIC handles a request to create a new network interface.
func (*Service) CreateNIC(c context.Context, req *crimson.CreateNICRequest) (*crimson.NIC, error) {
	if err := createNIC(c, req.Nic); err != nil {
		return nil, err
	}
	return req.Nic, nil
}

// DeleteNIC handles a request to delete an existing network interface.
func (*Service) DeleteNIC(c context.Context, req *crimson.DeleteNICRequest) (*empty.Empty, error) {
	if err := deleteNIC(c, req.Name, req.Machine); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// ListNICs handles a request to list network interfaces.
func (*Service) ListNICs(c context.Context, req *crimson.ListNICsRequest) (*crimson.ListNICsResponse, error) {
	nics, err := listNICs(c, database.Get(c), req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListNICsResponse{
		Nics: nics,
	}, nil
}

// UpdateNIC handles a request to update an existing network interface.
func (*Service) UpdateNIC(c context.Context, req *crimson.UpdateNICRequest) (*crimson.NIC, error) {
	return updateNIC(c, req.Nic, req.UpdateMask)
}

// createNIC creates a new NIC in the database.
func createNIC(c context.Context, n *crimson.NIC) error {
	if err := validateNICForCreation(n); err != nil {
		return err
	}
	mac, _ := common.ParseMAC48(n.MacAddress)
	tx, err := database.Begin(c)
	if err != nil {
		return errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	var hostnameID sql.NullInt64
	if n.Hostname != "" {
		ip, _ := common.ParseIPv4(n.Ipv4)
		id, err := model.AssignHostnameAndIP(c, tx, n.Hostname, ip)
		if err != nil {
			return err
		}
		hostnameID.Int64 = id
		hostnameID.Valid = true
	}

	// By setting nics.machine_id NOT NULL when setting up the database, we can avoid checking if the given machine is
	// valid. MySQL will turn up NULL for its column values which will be rejected as an error.
	_, err = tx.ExecContext(c, `
		INSERT INTO nics (name, machine_id, mac_address, switch_id, switchport, hostname_id)
		VALUES (?, (SELECT id FROM machines WHERE name = ?), ?, (SELECT id FROM switches WHERE name = ?), ?, ?)
	`, n.Name, n.Machine, mac, n.Switch, n.Switchport, hostnameID)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'name'"):
			// e.g. "Error 1062: Duplicate entry 'eth0-machineId' for key 'name'".
			return status.Errorf(codes.AlreadyExists, "duplicate NIC %q for machine %q", n.Name, n.Machine)
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'mac_address'"):
			// e.g. "Error 1062: Duplicate entry '1234567890' for key 'mac_address'".
			return status.Errorf(codes.AlreadyExists, "duplicate MAC address %q", n.MacAddress)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'machine_id' cannot be null".
			return status.Errorf(codes.NotFound, "machine %q does not exist", n.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'switch_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return status.Errorf(codes.NotFound, "switch %q does not exist", n.Switch)
		}
		return errors.Annotate(err, "failed to create NIC").Err()
	}

	if err := tx.Commit(); err != nil {
		return errors.Annotate(err, "failed to commit transaction").Err()
	}
	return nil
}

// getHostnameForNIC gets the ID of the hostname associated with an existing NIC.
func getHostnameForNIC(c context.Context, q database.QueryerContext, name, machine string) (*int64, error) {
	rows, err := q.QueryContext(c, `
		SELECT h.id FROM nics n, machines m, hostnames h
		WHERE n.machine_id = m.id AND n.hostname_id = h.id AND n.name = ? AND m.name = ?
	`, name, machine)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch associated hostname").Err()
	}
	defer rows.Close()
	if rows.Next() {
		var hostnameID int64
		if err = rows.Scan(&hostnameID); err != nil {
			return nil, errors.Annotate(err, "failed to fetch hostname").Err()
		}
		return &hostnameID, nil
	}
	return nil, nil
}

// deleteNIC deletes an existing NIC from the database.
func deleteNIC(c context.Context, name, machine string) error {
	switch {
	case name == "":
		return status.Error(codes.InvalidArgument, "NIC name is required and must be non-empty")
	case machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	}
	tx, err := database.Begin(c)
	if err != nil {
		return errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	// If a NIC is backing a host, don't delete it. If not, delete it and its hostname (if it has one).
	// Deleting a hostname cascades to the host, so hostname can't be deleted without first checking
	// for a host. Deleting a hostname sets null in the NIC, so the NIC still has to be deleted.
	hosts, err := listPhysicalHosts(c, tx, &crimson.ListPhysicalHostsRequest{
		Machines: []string{machine},
	})
	if err != nil {
		return errors.Annotate(err, "failed to fetch associated physical host").Err()
	}
	if len(hosts) > 0 {
		return status.Errorf(codes.FailedPrecondition, "delete entities referencing this NIC first")
	}

	// Delete the NIC's hostname, if it has one.
	hostnameID, err := getHostnameForNIC(c, tx, name, machine)
	if err != nil {
		return err
	}
	if hostnameID != nil {
		_, err = tx.ExecContext(c, `DELETE FROM hostnames WHERE id = ?`, *hostnameID)
		if err != nil {
			return errors.Annotate(err, "failed to delete associated hostname").Err()
		}
	}

	res, err := tx.ExecContext(c, `
		DELETE FROM nics WHERE name = ? AND machine_id = (SELECT id FROM machines WHERE name = ?)
	`, name, machine)
	if err != nil {
		return errors.Annotate(err, "failed to delete NIC").Err()
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return errors.Annotate(err, "failed to fetch affected rows").Err()
	case rows == 0:
		return status.Errorf(codes.NotFound, "NIC %q does not exist on machine %q", name, machine)
	}

	if err := tx.Commit(); err != nil {
		return errors.Annotate(err, "failed to commit transaction").Err()
	}
	return nil
}

// listNICs returns a slice of NICs in the database.
func listNICs(c context.Context, q database.QueryerContext, req *crimson.ListNICsRequest) ([]*crimson.NIC, error) {
	mac48s, err := parseMAC48s(req.MacAddresses)
	if err != nil {
		return nil, err
	}

	stmt := squirrel.Select("n.name", "m.name", "n.mac_address", "s.name", "n.switchport").
		From("nics n, machines m, switches s").
		Where("n.machine_id = m.id").Where("n.switch_id = s.id")
	stmt = selectInString(stmt, "n.name", req.Names)
	stmt = selectInString(stmt, "m.name", req.Machines)
	stmt = selectInUint64(stmt, "n.mac_address", mac48s)
	stmt = selectInString(stmt, "s.name", req.Switches)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	rows, err := q.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch NICs").Err()
	}
	defer rows.Close()
	var nics []*crimson.NIC
	for rows.Next() {
		n := &crimson.NIC{}
		var mac48 common.MAC48
		if err = rows.Scan(&n.Name, &n.Machine, &mac48, &n.Switch, &n.Switchport); err != nil {
			return nil, errors.Annotate(err, "failed to fetch NIC").Err()
		}
		n.MacAddress = mac48.String()
		nics = append(nics, n)
	}
	return nics, nil
}

// parseMAC48s returns a slice of uint64 MAC addresses.
func parseMAC48s(macs []string) ([]uint64, error) {
	mac48s := make([]uint64, len(macs))
	for i, mac := range macs {
		mac48, err := common.ParseMAC48(mac)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", mac)
		}
		mac48s[i] = uint64(mac48)
	}
	return mac48s, nil
}

// updateNIC updates an existing NIC in the database.
func updateNIC(c context.Context, n *crimson.NIC, mask *field_mask.FieldMask) (*crimson.NIC, error) {
	if err := validateNICForUpdate(n, mask); err != nil {
		return nil, err
	}
	stmt := squirrel.Update("nics")
	for _, path := range mask.Paths {
		switch path {
		case "mac_address":
			mac, _ := common.ParseMAC48(n.MacAddress)
			stmt = stmt.Set("mac_address", mac)
		case "switch":
			stmt = stmt.Set("switch_id", squirrel.Expr("(SELECT id FROM switches WHERE name = ?)", n.Switch))
		case "switchport":
			stmt = stmt.Set("switchport", n.Switchport)
		}
	}
	stmt = stmt.Where("name = ?", n.Name).Where("machine_id = (SELECT id FROM machines WHERE name = ?)", n.Machine)
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
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'mac_address'"):
			// e.g. "Error 1062: Duplicate entry '1234567890' for key 'mac_address'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate MAC address %q", n.MacAddress)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'switch_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "switch %q does not exist", n.Switch)
		}
		return nil, errors.Annotate(err, "failed to update NIC").Err()
	}
	// The number of rows affected cannot distinguish between zero because the NIC didn't exist
	// and zero because the row already matched, so skip looking at the number of rows affected.

	nics, err := listNICs(c, tx, &crimson.ListNICsRequest{
		Names:    []string{n.Name},
		Machines: []string{n.Machine},
	})
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch updated NIC").Err()
	case len(nics) == 0:
		return nil, status.Errorf(codes.NotFound, "NIC %q does not exist on machine %q", n.Name, n.Machine)
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return nics[0], nil
}

// validateNICForCreation validates a NIC for creation.
func validateNICForCreation(n *crimson.NIC) error {
	switch {
	case n == nil:
		return status.Error(codes.InvalidArgument, "NIC specification is required")
	case n.Name == "":
		return status.Error(codes.InvalidArgument, "NIC name is required and must be non-empty")
	case n.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case n.Switch == "":
		return status.Error(codes.InvalidArgument, "switch is required and must be non-empty")
	case n.Switchport < 1:
		return status.Error(codes.InvalidArgument, "switchport must be positive")
	default:
		// If hostname or IPv4 address is specified, require both.
		if n.Hostname != "" || n.Ipv4 != "" {
			if n.Hostname == "" {
				return status.Errorf(codes.InvalidArgument, "if IPv4 is specified then hostname is required and must be non-empty")
			}
			if n.Ipv4 == "" {
				return status.Errorf(codes.InvalidArgument, "if hostname is specified then IPv4 address is required and must be non-empty")
			}
			_, err := common.ParseIPv4(n.Ipv4)
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid IPv4 address %q", n.Ipv4)
			}
		}
		_, err := common.ParseMAC48(n.MacAddress)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", n.MacAddress)
		}
		return nil
	}
}

// validateNICForUpdate validates a NIC for update.
func validateNICForUpdate(n *crimson.NIC, mask *field_mask.FieldMask) error {
	switch err := validateUpdateMask(mask); {
	case n == nil:
		return status.Error(codes.InvalidArgument, "NIC specification is required")
	case n.Name == "":
		return status.Error(codes.InvalidArgument, "NIC name is required and must be non-empty")
	case n.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case err != nil:
		return err
	}
	for _, path := range mask.Paths {
		// TODO(smut): Allow hostname, IPv4 address to be updated.
		switch path {
		case "name":
			return status.Error(codes.InvalidArgument, "NIC name cannot be updated, delete and create a new NIC instead")
		case "machine":
			return status.Error(codes.InvalidArgument, "machine cannot be updated, delete and create a new NIC instead")
		case "mac_address":
			if n.MacAddress == "" {
				return status.Error(codes.InvalidArgument, "MAC address is required and must be non-empty")
			}
			_, err := common.ParseMAC48(n.MacAddress)
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", n.MacAddress)
			}
		case "switch":
			if n.Switch == "" {
				return status.Error(codes.InvalidArgument, "switch is required and must be non-empty")
			}
		case "switchport":
			if n.Switchport < 1 {
				return status.Error(codes.InvalidArgument, "switchport must be positive")
			}
		default:
			return status.Errorf(codes.InvalidArgument, "unsupported update mask path %q", path)
		}
	}
	return nil
}
