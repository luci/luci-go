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

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Masterminds/squirrel"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/errors"

	states "go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// CreatePhysicalHost handles a request to create a new physical host.
func (*Service) CreatePhysicalHost(c context.Context, req *crimson.CreatePhysicalHostRequest) (*crimson.PhysicalHost, error) {
	if err := createPhysicalHost(c, req.Host); err != nil {
		return nil, err
	}
	return req.Host, nil
}

// ListPhysicalHosts handles a request to list physical hosts.
func (*Service) ListPhysicalHosts(c context.Context, req *crimson.ListPhysicalHostsRequest) (*crimson.ListPhysicalHostsResponse, error) {
	hosts, err := listPhysicalHosts(c, database.Get(c), req)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListPhysicalHostsResponse{
		Hosts: hosts,
	}, nil
}

// UpdatePhysicalHost handles a request to update an existing physical host.
func (*Service) UpdatePhysicalHost(c context.Context, req *crimson.UpdatePhysicalHostRequest) (*crimson.PhysicalHost, error) {
	host, err := updatePhysicalHost(c, req.Host, req.UpdateMask)
	if err != nil {
		return nil, err
	}
	return host, nil
}

// createPhysicalHost creates a new physical host in the database.
func createPhysicalHost(c context.Context, h *crimson.PhysicalHost) (err error) {
	if err := validatePhysicalHostForCreation(h); err != nil {
		return err
	}
	ip, _ := common.ParseIPv4(h.Ipv4)
	tx, err := database.Begin(c)
	if err != nil {
		return internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}
	defer func() {
		if err != nil {
			if e := tx.Rollback(); e != nil {
				errors.Log(c, errors.Annotate(e, "failed to roll back transaction").Err())
			}
		}
	}()

	hostnameId, err := assignHostnameAndIP(c, tx, h.Name, ip)
	if err != nil {
		return err
	}

	// physical_hosts.hostname_id, physical_hosts.machine_id, and physical_hosts.os_id are NOT NULL as above.
	_, err = tx.ExecContext(c, `
		INSERT INTO physical_hosts (hostname_id, machine_id, os_id, vm_slots, description, deployment_ticket)
		VALUES (
			?,
			(SELECT id FROM machines WHERE name = ?),
			(SELECT id FROM oses WHERE name = ?),
			?,
			?,
			?
		)
	`, hostnameId, h.Machine, h.Os, h.VmSlots, h.Description, h.DeploymentTicket)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
			return status.Errorf(codes.AlreadyExists, "duplicate physical host for machine %q", h.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
			// e.g. "Error 1048: Column 'machine_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown machine %q", h.Machine)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'os_id'"):
			// e.g. "Error 1048: Column 'os_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown operating system %q", h.Os)
		}
		return internalError(c, errors.Annotate(err, "failed to create physical host").Err())
	}

	// Physical host state is stored with the backing machine. Update if necessary.
	if h.State != states.State_STATE_UNSPECIFIED {
		_, err := tx.ExecContext(c, `
			UPDATE machines
			SET state = ?
			WHERE name = ?
		`, h.State, h.Machine)
		if err != nil {
			return internalError(c, errors.Annotate(err, "failed to update machine").Err())
		}
	}

	if err := tx.Commit(); err != nil {
		return internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return nil
}

// listPhysicalHosts returns a slice of physical hosts in the database.
func listPhysicalHosts(c context.Context, q database.QueryerContext, req *crimson.ListPhysicalHostsRequest) ([]*crimson.PhysicalHost, error) {
	ipv4s, err := parseIPv4s(req.Ipv4S)
	if err != nil {
		return nil, err
	}

	stmt := squirrel.Select(
		"h.name",
		"v.id",
		"m.name",
		"o.name",
		"p.vm_slots",
		"p.description",
		"p.deployment_ticket",
		"i.ipv4",
		"m.state",
	)
	stmt = stmt.From("physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i").
		Where("p.hostname_id = h.id").
		Where("h.vlan_id = v.id").
		Where("p.machine_id = m.id").
		Where("p.os_id = o.id").
		Where("i.hostname_id = h.id")
	stmt = selectInString(stmt, "h.name", req.Names)
	stmt = selectInInt64(stmt, "v.id", req.Vlans)
	stmt = selectInInt64(stmt, "i.ipv4", ipv4s)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to generate statement").Err())
	}

	rows, err := q.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch physical hosts").Err()
	}
	defer rows.Close()
	var hosts []*crimson.PhysicalHost
	for rows.Next() {
		h := &crimson.PhysicalHost{}
		var ipv4 common.IPv4
		if err = rows.Scan(
			&h.Name,
			&h.Vlan,
			&h.Machine,
			&h.Os,
			&h.VmSlots,
			&h.Description,
			&h.DeploymentTicket,
			&ipv4,
			&h.State,
		); err != nil {
			return nil, errors.Annotate(err, "failed to fetch physical host").Err()
		}
		h.Ipv4 = ipv4.String()
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// updatePhysicalHost updates an existing physical host in the database.
func updatePhysicalHost(c context.Context, h *crimson.PhysicalHost, mask *field_mask.FieldMask) (_ *crimson.PhysicalHost, err error) {
	if err := validatePhysicalHostForUpdate(h, mask); err != nil {
		return nil, err
	}
	update := false
	updateState := false
	stmt := squirrel.Update("physical_hosts")
	for _, path := range mask.Paths {
		switch path {
		case "machine":
			stmt = stmt.Set("machine_id", squirrel.Expr("(SELECT id FROM machines WHERE name = ?)", h.Machine))
			update = true
		case "os":
			stmt = stmt.Set("os_id", squirrel.Expr("(SELECT id FROM oses WHERE name = ?)", h.Os))
			update = true
		case "state":
			updateState = true
		case "vm_slots":
			stmt = stmt.Set("vm_slots", h.VmSlots)
			update = true
		case "description":
			stmt = stmt.Set("description", h.Description)
			update = true
		case "deployment_ticket":
			stmt = stmt.Set("deployment_ticket", h.DeploymentTicket)
			update = true
		}
	}
	var query string
	var args []interface{}
	if update {
		stmt = stmt.Where("hostname_id = (SELECT id FROM hostnames WHERE name = ? AND vlan_id = ?)", h.Name, h.Vlan)
		query, args, err = stmt.ToSql()
		if err != nil {
			return nil, internalError(c, errors.Annotate(err, "failed to generate statement").Err())
		}
	}

	tx, err := database.Begin(c)
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to begin transaction").Err())
	}
	defer func() {
		if err != nil {
			if e := tx.Rollback(); e != nil {
				errors.Log(c, errors.Annotate(e, "failed to roll back transaction").Err())
			}
		}
	}()
	if query != "" && len(args) > 0 {
		_, err = tx.ExecContext(c, query, args...)
		if err != nil {
			switch e, ok := err.(*mysql.MySQLError); {
			case !ok:
				// Type assertion failed.
			case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "'machine_id'"):
				// e.g. "Error 1062: Duplicate entry '1' for key 'machine_id'".
				return nil, status.Errorf(codes.AlreadyExists, "duplicate physical host for machine %q", h.Machine)
			case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'machine_id'"):
				// e.g. "Error 1048: Column 'machine_id' cannot be null".
				return nil, status.Errorf(codes.NotFound, "unknown machine %q", h.Machine)
			case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'os_id'"):
				// e.g. "Error 1048: Column 'os_id' cannot be null".
				return nil, status.Errorf(codes.NotFound, "unknown operating system %q", h.Os)
			}
			return nil, internalError(c, errors.Annotate(err, "failed to update physical host").Err())
		}
		// The number of rows affected cannot distinguish between zero because the physical host didn't exist
		// and zero because the row already matched, so skip looking at the number of rows affected.
	}
	if updateState {
		_, err = tx.ExecContext(c, `
			UPDATE machines
			SET state = ?
			WHERE id = (SELECT machine_id FROM physical_hosts WHERE hostname_id = (SELECT id FROM hostnames WHERE name = ? AND vlan_id = ?))
		`, h.State, h.Name, h.Vlan)
		if err != nil {
			return nil, internalError(c, errors.Annotate(err, "failed to update machine").Err())
		}
	}

	hosts, err := listPhysicalHosts(c, tx, &crimson.ListPhysicalHostsRequest{
		Names: []string{h.Name},
		Vlans: []int64{h.Vlan},
	})
	switch {
	case err != nil:
		return nil, internalError(c, errors.Annotate(err, "failed to fetch updated physical host").Err())
	case len(hosts) == 0:
		return nil, status.Errorf(codes.NotFound, "unknown physical host %q for VLAN %d", h.Name, h.Vlan)
	}

	if err := tx.Commit(); err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return hosts[0], nil
}

// validatePhysicalHostForCreation validates a physical host for creation.
func validatePhysicalHostForCreation(h *crimson.PhysicalHost) error {
	switch {
	case h == nil:
		return status.Error(codes.InvalidArgument, "physical host specification is required")
	case h.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case h.Vlan != 0:
		return status.Error(codes.InvalidArgument, "VLAN must not be specified, use IP address instead")
	case h.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case h.Os == "":
		return status.Error(codes.InvalidArgument, "operating system is required and must be non-empty")
	case h.VmSlots < 0:
		return status.Error(codes.InvalidArgument, "VM slots must be non-negative")
	default:
		_, err := common.ParseIPv4(h.Ipv4)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid IPv4 address %q", h.Ipv4)
		}
		return nil
	}
}

// validatePhysicalHostForUpdate validates a physical host for update.
func validatePhysicalHostForUpdate(h *crimson.PhysicalHost, mask *field_mask.FieldMask) error {
	switch err := validateUpdateMask(mask); {
	case h == nil:
		return status.Error(codes.InvalidArgument, "physical host specification is required")
	case h.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case h.Vlan < 1:
		return status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	case err != nil:
		return err
	}
	for _, path := range mask.Paths {
		// TODO(smut): Allow IPv4 address and state to be updated.
		switch path {
		case "name":
			return status.Error(codes.InvalidArgument, "hostname cannot be updated, delete and create a new physical host instead")
		case "vlan":
			return status.Error(codes.InvalidArgument, "VLAN cannot be updated, delete and create a new physical host instead")
		case "machine":
			if h.Machine == "" {
				return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
			}
		case "os":
			if h.Os == "" {
				return status.Error(codes.InvalidArgument, "operating system is required and must be non-empty")
			}
		case "state":
			if h.State == states.State_STATE_UNSPECIFIED {
				return status.Error(codes.InvalidArgument, "state is required")
			}
		case "vm_slots":
			if h.VmSlots < 0 {
				return status.Error(codes.InvalidArgument, "VM slots must be non-negative")
			}
		case "description":
			// Empty description is allowed, nothing to validate.
		case "deployment_ticket":
			// Empty deployment ticket is allowed, nothing to validate.
		default:
			return status.Errorf(codes.InvalidArgument, "unsupported update mask path %q", path)
		}
	}
	return nil
}
