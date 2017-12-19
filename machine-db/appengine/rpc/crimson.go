// Copyright 2017 The LUCI Authors.
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
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// internalError logs and returns an internal gRPC error.
func internalError(c context.Context, err error) error {
	errors.Log(c, err)
	return status.Errorf(codes.Internal, "Internal server error")
}

// isAuthorized returns whether the current user is authorized to use the Crimson API.
func isAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "machine-db-administrators")
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership").Err()
	}
	return is, nil
}

// authPrelude ensure the user is authorized to use the Crimson API.
func authPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch authorized, err := isAuthorized(c); {
	case err != nil:
		return c, internalError(c, err)
	case !authorized:
		return c, status.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	return c, nil
}

// NewServer returns a new Crimson RPC server.
func NewServer() crimson.CrimsonServer {
	return &crimson.DecoratedCrimson{
		Prelude: authPrelude,
		Service: &Service{},
	}
}

// Service handles Crimson RPCs.
type Service struct {
}

// Returns whether the given string matches the given set.
// An empty set matches all strings.
func matches(s string, set stringset.Set) bool {
	return set.Has(s) || set.Len() == 0
}

// GetDatacenters handles a request to retrieve datacenters.
func (*Service) GetDatacenters(c context.Context, req *crimson.DatacentersRequest) (*crimson.DatacentersResponse, error) {
	datacenters, err := getDatacenters(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.DatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// getDatacenters returns a slice of datacenters in the database.
func getDatacenters(c context.Context, names stringset.Set) ([]*crimson.Datacenter, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT d.name, d.description
		FROM datacenters d
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	var datacenters []*crimson.Datacenter
	for rows.Next() {
		dc := &crimson.Datacenter{}
		if err = rows.Scan(&dc.Name, &dc.Description); err != nil {
			return nil, errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if matches(dc.Name, names) {
			datacenters = append(datacenters, dc)
		}
	}
	return datacenters, nil
}

// GetRacks handles a request to retrieve racks.
func (*Service) GetRacks(c context.Context, req *crimson.RacksRequest) (*crimson.RacksResponse, error) {
	racks, err := getRacks(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Datacenters...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.RacksResponse{
		Racks: racks,
	}, nil
}

// getRacks returns a slice of racks in the database.
func getRacks(c context.Context, names, datacenters stringset.Set) ([]*crimson.Rack, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT r.name, r.description, d.name
		FROM racks r, datacenters d
		WHERE r.datacenter_id = d.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch racks").Err()
	}
	defer rows.Close()

	var racks []*crimson.Rack
	for rows.Next() {
		rack := &crimson.Rack{}
		if err = rows.Scan(&rack.Name, &rack.Description, &rack.Datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch rack").Err()
		}
		if matches(rack.Name, names) && matches(rack.Datacenter, datacenters) {
			racks = append(racks, rack)
		}
	}
	return racks, nil
}

// GetSwitches handles a request to retrieve switches.
func (*Service) GetSwitches(c context.Context, req *crimson.SwitchesRequest) (*crimson.SwitchesResponse, error) {
	switches, err := getSwitches(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Racks...), stringset.NewFromSlice(req.Datacenters...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.SwitchesResponse{
		Switches: switches,
	}, nil
}

// getSwitches returns a slice of switches in the database.
func getSwitches(c context.Context, names, racks, datacenters stringset.Set) ([]*crimson.Switch, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT s.name, s.description, s.ports, r.name, d.name
		FROM switches s, racks r, datacenters d
		WHERE s.rack_id = r.id
			AND r.datacenter_id = d.id
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch switches").Err()
	}
	defer rows.Close()

	var switches []*crimson.Switch
	for rows.Next() {
		s := &crimson.Switch{}
		if err = rows.Scan(&s.Name, &s.Description, &s.Ports, &s.Rack, &s.Datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch switch").Err()
		}
		if matches(s.Name, names) && matches(s.Rack, racks) && matches(s.Datacenter, datacenters) {
			switches = append(switches, s)
		}
	}
	return switches, nil
}
