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

	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// CreateVM handles a request to create a new VM.
func (*Service) CreateVM(c context.Context, req *crimson.CreateVMRequest) (*crimson.VM, error) {
	if err := createVM(c, req.Vm); err != nil {
		return nil, err
	}
	return req.Vm, nil
}

// ListVMs handles a request to list VMs.
func (*Service) ListVMs(c context.Context, req *crimson.ListVMsRequest) (*crimson.ListVMsResponse, error) {
	vlans := make(map[int64]struct{}, len(req.Vlans))
	for _, vlan := range req.Vlans {
		vlans[vlan] = struct{}{}
	}
	vms, err := listVMs(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Hosts...), vlans)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListVMsResponse{
		Vms: vms,
	}, nil
}

// createVM creates a new VM in the database. Returns a gRPC error if unsuccessful.
func createVM(c context.Context, vm *crimson.VM) error {
	if err := validateVMForCreation(vm); err != nil {
		return err
	}
	tx, err := database.Begin(c)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}

	// TODO(smut): Check that the provided IP address is unassigned.
	// TODO(smut): Check that the provided host has sufficient available VM slots.

	// By setting vms.host_id, vms.vlan_id, and vms.os_id NOT NULL when setting up the database, we can avoid checking
	// if the given host, VLAN, and OS are valid. MySQL will turn up NULL for its column values which will be rejected
	// as an error. We don't need to look up VLAN because the ID is given in the request, but we look it up anyway to
	// ensure it exists in the database.
	stmt, err := tx.PrepareContext(c, `
		INSERT INTO vms (name, host_id, vlan_id, os_id, description, deployment_ticket)
		VALUES (?, (SELECT id FROM hosts WHERE name = ? AND vlan_id = ?), (SELECT id FROM vlans WHERE id = ?), (SELECT id FROM oses WHERE name = ?), ?, ?)
	`)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to prepare statement").Err())
	}
	_, err = stmt.ExecContext(c, vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'name'"):
			// e.g. "Error 1062: Duplicate entry 'vm-host' for key 'name'".
			return status.Errorf(codes.AlreadyExists, "duplicate VM %q for host %q", vm.Name, vm.Host)
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'name_2'"):
			// e.g. "Error 1062: Duplicate entry 'vm-vlan' for key 'name_2'".
			return status.Errorf(codes.AlreadyExists, "duplicate VM %q for VLAN %d", vm.Name, vm.Vlan)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'host_id'"):
			// e.g. "Error 1048: Column 'host_id' cannot be null".
			// The same error message may indicate that either host or VLAN is invalid because VLAN is used to look up host ID.
			return status.Errorf(codes.NotFound, "unknown host %q for VLAN %d", vm.Host, vm.Vlan)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'vlan_id'"):
			// e.g. "Error 1048: Column 'vlan_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown VLAN %d", vm.Vlan)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'os_id'"):
			// e.g. "Error 1048: Column 'os_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown operating system %q", vm.Os)
		}
		return internalError(c, errors.Annotate(err, "failed to create VM").Err())
	}

	// TODO(smut): Assign the provided IP address.

	if err := tx.Commit(); err != nil {
		return internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return nil
}

// listVMs returns a slice of VMs in the database.
func listVMs(c context.Context, names stringset.Set, hosts stringset.Set, vlans map[int64]struct{}) ([]*crimson.VM, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT v.name, h.name, h.vlan_id, o.name, v.description, v.deployment_ticket
		FROM vms v, hosts h, oses o
		WHERE v.host_id = h.id
			AND h.os_id = o.id
	`)
	// TODO(smut): Fetch the assigned IP address.
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch VMs").Err()
	}
	defer rows.Close()

	var vms []*crimson.VM
	for rows.Next() {
		vm := &crimson.VM{}
		if err = rows.Scan(&vm.Name, &vm.Host, &vm.Vlan, &vm.Os, &vm.Description, &vm.DeploymentTicket); err != nil {
			return nil, errors.Annotate(err, "failed to fetch VM").Err()
		}
		// TODO(smut): use the database to filter rather than fetching all entries.
		if _, ok := vlans[vm.Vlan]; matches(vm.Name, names) && matches(vm.Host, hosts) && (ok || len(vlans) == 0) {
			vms = append(vms, vm)
		}
	}
	return vms, nil
}

// validateVMForCreation validates a VM for creation.
func validateVMForCreation(vm *crimson.VM) error {
	switch {
	case vm == nil:
		return status.Error(codes.InvalidArgument, "VM specification is required")
	case vm.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case vm.Host == "":
		return status.Error(codes.InvalidArgument, "physical host is required and must be non-empty")
	case vm.Vlan < 1:
		return status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	case vm.Os == "":
		return status.Error(codes.InvalidArgument, "operating system is required and must be non-empty")
	default:
		return nil
	}
}
