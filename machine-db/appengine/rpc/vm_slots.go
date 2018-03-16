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
	rows, err := q.QueryContext(c, `
		SELECT h.name, h.vlan_id, p.vm_slots - COUNT(v.physical_host_id)
		FROM (physical_hosts p, hostnames h)
		LEFT JOIN vms v on v.physical_host_id = p.id
		WHERE p.hostname_id = h.id
			AND p.vm_slots > 0
		GROUP BY h.name, h.vlan_id, p.vm_slots
		HAVING p.vm_slots > COUNT(v.physical_host_id)
		LIMIT ?
	`, req.Slots)
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to fetch VM slots").Err())
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
			return nil, internalError(c, errors.Annotate(err, "failed to fetch VM slots").Err())
		}
		hosts = append(hosts, h)
		slots += h.VmSlots
	}
	return hosts, nil
}
