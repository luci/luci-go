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

// CreateHost handles a request to create a new host.
func (*Service) CreateHost(c context.Context, req *crimson.CreateHostRequest) (*crimson.Host, error) {
	if err := createHost(c, req.Host); err != nil {
		return nil, err
	}
	return req.Host, nil
}

// ListHosts handles a request to list hosts.
func (*Service) ListHosts(c context.Context, req *crimson.ListHostsRequest) (*crimson.ListHostsResponse, error) {
	vlans := make(map[int64]struct{}, len(req.Vlans))
	for _, vlan := range req.Vlans {
		vlans[vlan] = struct{}{}
	}
	hosts, err := listHosts(c, stringset.NewFromSlice(req.Names...), vlans)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListHostsResponse{
		Hosts: hosts,
	}, nil
}

// createHost creates a new host in the database. Returns a gRPC error if unsuccessful.
func createHost(c context.Context, h *crimson.Host) error {
	if err := validateHostForCreation(h); err != nil {
		return err
	}
	tx, err := database.Begin(c)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}

	// TODO(smut): Check that the provided IP address is unassigned.

	// By setting hosts.vlan_id, hosts.machine_id, and hosts.os_id NOT NULL when setting up the database, we can avoid
	// checking if the given VLAN, machine, and OS are valid. MySQL will turn up NULL for its column values which will be
	// rejected as an error. We don't need to look up VLAN because the ID is given in the request, but we look it up
	// anyway to ensure it exists in the database.
	stmt, err := tx.PrepareContext(c, `
		INSERT INTO hosts (name, vlan_id, machine_id, os_id, vm_slots, description, deployment_ticket)
		VALUES (?, (SELECT id FROM vlans WHERE id = ?), (SELECT id FROM machines WHERE name = ?), (SELECT id FROM oses WHERE name = ?), ?, ?, ?)
	`)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to prepare statement").Err())
	}
	_, err = stmt.ExecContext(c, h.Name, h.Vlan, h.Machine, h.Os, h.VmSlots, h.Description, h.DeploymentTicket)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'name'"):
			// e.g. "Error 1062: Duplicate entry 'hostname-vlanId' for key 'name'".
			return status.Errorf(codes.AlreadyExists, "duplicate host %q for VLAN %d", h.Name, h.Vlan)
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
			return status.Errorf(codes.AlreadyExists, "duplicate host for machine %q", h.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'vlan_id'"):
			// e.g. "Error 1048: Column 'vlan_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown VLAN %d", h.Vlan)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'machine_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown machine %q", h.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'os_id'"):
			// e.g. "Error 1048: Column 'os_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown operating system %q", h.Os)
		}
		return internalError(c, errors.Annotate(err, "failed to create host").Err())
	}

	// TODO(smut): Assign the provided IP address.

	if err := tx.Commit(); err != nil {
		return internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return nil
}

// listHosts returns a slice of hosts in the database.
func listHosts(c context.Context, names stringset.Set, vlans map[int64]struct{}) ([]*crimson.Host, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT h.name, v.id, m.name, o.name, h.vm_slots, h.description, h.deployment_ticket
		FROM hosts h, vlans v, machines m, oses o
		WHERE h.vlan_id = v.id
			AND h.machine_id = m.id
			AND h.os_id = o.id
	`)
	// TODO(smut): Fetch the assigned IP address.
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch hosts").Err()
	}
	defer rows.Close()

	var hosts []*crimson.Host
	for rows.Next() {
		h := &crimson.Host{}
		if err = rows.Scan(&h.Name, &h.Vlan, &h.Machine, &h.Os, &h.VmSlots, &h.Description, &h.DeploymentTicket); err != nil {
			return nil, errors.Annotate(err, "failed to fetch host").Err()
		}
		// TODO(smut): use the database to filter rather than fetching all entries.
		if _, ok := vlans[h.Vlan]; matches(h.Name, names) && (ok || len(vlans) == 0) {
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// validateHostForCreation validates a host for creation.
func validateHostForCreation(h *crimson.Host) error {
	switch {
	case h == nil:
		return status.Error(codes.InvalidArgument, "host specification is required")
	case h.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case h.Vlan < 1:
		return status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	case h.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case h.Os == "":
		return status.Error(codes.InvalidArgument, "operating system is required and must be non-empty")
	default:
		return nil
	}
}
