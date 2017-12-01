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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// internalError logs and returns an internal gRPC error.
func internalError(c context.Context, err error) error {
	errors.Log(c, err)
	return grpc.Errorf(codes.Internal, "Internal server error")
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
		return c, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
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
	rows, err := db.QueryContext(c, "SELECT name, description FROM datacenters")
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
		if names.Has(dc.Name) || names.Len() == 0 {
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
	rows, err := db.QueryContext(c, "SELECT racks.name, racks.description, datacenters.name FROM racks, datacenters WHERE racks.datacenter_id = datacenters.id")
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
		if (names.Has(rack.Name) || names.Len() == 0) && (datacenters.Has(rack.Datacenter) || datacenters.Len() == 0) {
			racks = append(racks, rack)
		}
	}
	return racks, nil
}
