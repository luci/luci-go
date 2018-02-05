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
	vlans := make(map[int64]struct{}, len(req.Vlans))
	for _, vlan := range req.Vlans {
		vlans[vlan] = struct{}{}
	}
	hosts, err := listPhysicalHosts(c, stringset.NewFromSlice(req.Names...), vlans)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListPhysicalHostsResponse{
		Hosts: hosts,
	}, nil
}

// createPhysicalHost creates a new physical host in the database. Returns a gRPC error if unsuccessful.
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

	hostnameId, err := assignHostnameAndIP(c, tx, h.Name, h.Vlan, ip)
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

	if err := tx.Commit(); err != nil {
		return internalError(c, errors.Annotate(err, "failed to commit transaction").Err())
	}
	return nil
}

// listPhysicalHosts returns a slice of physical hosts in the database.
func listPhysicalHosts(c context.Context, names stringset.Set, vlans map[int64]struct{}) ([]*crimson.PhysicalHost, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
		FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
		WHERE p.hostname_id = h.id
			AND h.vlan_id = v.id
			AND p.machine_id = m.id
			AND p.os_id = o.id
			AND i.hostname_id = h.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch physical hosts").Err()
	}
	defer rows.Close()

	var hosts []*crimson.PhysicalHost
	for rows.Next() {
		h := &crimson.PhysicalHost{}
		var ipv4 common.IPv4
		if err = rows.Scan(&h.Name, &h.Vlan, &h.Machine, &h.Os, &h.VmSlots, &h.Description, &h.DeploymentTicket, &ipv4); err != nil {
			return nil, errors.Annotate(err, "failed to fetch physical host").Err()
		}
		// TODO(smut): use the database to filter rather than fetching all entries.
		if _, ok := vlans[h.Vlan]; matches(h.Name, names) && (ok || len(vlans) == 0) {
			h.Ipv4 = ipv4.String()
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// validatePhysicalHostForCreation validates a physical host for creation.
func validatePhysicalHostForCreation(h *crimson.PhysicalHost) error {
	switch {
	case h == nil:
		return status.Error(codes.InvalidArgument, "physical host specification is required")
	case h.Name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case h.Vlan < 1:
		return status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	case h.Machine == "":
		return status.Error(codes.InvalidArgument, "machine is required and must be non-empty")
	case h.Os == "":
		return status.Error(codes.InvalidArgument, "operating system is required and must be non-empty")
	case h.Ipv4 == "":
		return status.Error(codes.InvalidArgument, "IPv4 address is required and must be non-empty")
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
