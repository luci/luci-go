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
	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Masterminds/squirrel"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// maxVMSlots is the maximum number of available VM slots which can be found at a time.
const maxVMSlots = 32

// FindVMSlots handles a request to find available VM slots.
func (*Service) FindVMSlots(c context.Context, req *crimson.FindVMSlotsRequest) (*crimson.FindVMSlotsResponse, error) {
	hosts, err := findVMSlots(c, database.Get(c), req)
	if err != nil {
		return nil, err
	}
	return &crimson.FindVMSlotsResponse{
		Hosts: hosts,
	}, nil
}

// findVMSlots returns a slice of physical hosts with available VM slots in the database.
func findVMSlots(c context.Context, q database.QueryerContext, req *crimson.FindVMSlotsRequest) ([]*crimson.PhysicalHost, error) {
	switch {
	case req.Slots < 1:
		return nil, status.Error(codes.InvalidArgument, "slots is required and must be positive")
	case req.Slots > maxVMSlots:
		return nil, status.Errorf(codes.InvalidArgument, "slots must not exceed %d", maxVMSlots)
	}
	stmt := squirrel.Select("h.name", "h.vlan_id", "ph.vm_slots - COUNT(v.physical_host_id)").
		From("(physical_hosts ph, hostnames h)")
	if len(req.Manufacturers) > 0 {
		stmt = stmt.Join("machines m ON ph.machine_id = m.id")
		stmt = stmt.Join("platforms pl ON m.platform_id = pl.id")
	}
	stmt = stmt.LeftJoin("vms v on v.physical_host_id = ph.id").
		Where("ph.hostname_id = h.id").
		Where("ph.vm_slots > 0").
		GroupBy("h.name", "h.vlan_id", "ph.vm_slots").
		Having("ph.vm_slots > COUNT(v.physical_host_id)").
		// In the worst case, each host with at least one available VM slot has only one available VM slot.
		// Set the limit to assume the worst and refine the result later.
		Limit(uint64(req.Slots))
	stmt = selectInString(stmt, "pl.manufacturer", req.Manufacturers)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}
	rows, err := q.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch VM slots").Err()
	}
	defer rows.Close()
	var hosts []*crimson.PhysicalHost
	var slots int32
	for rows.Next() && slots < req.Slots {
		h := &crimson.PhysicalHost{}
		if err = rows.Scan(
			&h.Name,
			&h.Vlan,
			&h.VmSlots,
		); err != nil {
			return nil, errors.Annotate(err, "failed to fetch VM slots").Err()
		}
		hosts = append(hosts, h)
		slots += h.VmSlots
	}
	return hosts, nil
}
