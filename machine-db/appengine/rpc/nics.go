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

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
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
	nics, err := listNICs(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Machines...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListNICsResponse{
		Nics: nics,
	}, nil
}

// createNIC creates a new NIC in the database.
func createNIC(c context.Context, n *crimson.NIC) error {
	if err := validateNICForCreation(n); err != nil {
		return err
	}
	mac, err := common.ParseMAC48(n.MacAddress)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid MAC-48 address %q", n.MacAddress)
	}
	tx, err := database.Begin(c)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}

	// By setting nics.machine_id NOT NULL when setting up the database, we can avoid checking if the given machine is
	// valid. MySQL will turn up NULL for its column values which will be rejected as an error.
	stmt, err := tx.PrepareContext(c, `
		INSERT INTO nics (name, machine_id, mac_address, switch_id, switchport)
		VALUES (?, (SELECT id FROM machines WHERE name = ?), ?, (SELECT id FROM switches WHERE name = ?), ?)
	`)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to prepare statement").Err())
	}
	_, err = stmt.ExecContext(c, n.Name, n.Machine, mac, n.Switch, n.Switchport)
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
			return status.Errorf(codes.NotFound, "unknown machine %q", n.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'switch_id'"):
			// e.g. "Error 1048: Column 'switch_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown switch %q", n.Switch)
		}
		return internalError(c, errors.Annotate(err, "failed to create NIC").Err())
	}

	if err := tx.Commit(); err != nil {
		return internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return nil
}

// deleteNIC deletes an existing NIC from the database.
func deleteNIC(c context.Context, name, machine string) error {
	switch {
	case name == "":
		return status.Error(codes.InvalidArgument, "NIC name is required and must be non-empty")
	case machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM nics WHERE name = ? AND machine_id = (SELECT id FROM machines WHERE name = ?)
	`)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to prepare statement").Err())
	}
	res, err := stmt.ExecContext(c, name, machine)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to delete NIC").Err())
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return internalError(c, errors.Annotate(err, "failed to fetch rows").Err())
	case rows == 0:
		return status.Errorf(codes.NotFound, "unknown NIC %q for machine %q", name, machine)
	case rows == 1:
		return nil
	default:
		// Shouldn't happen because name is unique in the database.
		return internalError(c, errors.Annotate(err, "unexpected number of affected rows %d", rows).Err())
	}
}

// listNICs returns a slice of NICs in the database.
func listNICs(c context.Context, names stringset.Set, machines stringset.Set) ([]*crimson.NIC, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT n.name, m.name, n.mac_address, s.name, n.switchport
		FROM nics n, machines m, switches s
		WHERE n.machine_id = m.id
			AND n.switch_id = s.id
	`)
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
		// TODO(smut): use the database to filter rather than fetching all entries.
		if matches(n.Name, names) && matches(n.Machine, machines) {
			n.MacAddress = mac48.String()
			nics = append(nics, n)
		}
	}
	return nics, nil
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
	case n.MacAddress == "":
		return status.Error(codes.InvalidArgument, "MAC address is required and must be non-empty")
	case n.Switch == "":
		return status.Error(codes.InvalidArgument, "switch is required and must be non-empty")
	default:
		return nil
	}
}
