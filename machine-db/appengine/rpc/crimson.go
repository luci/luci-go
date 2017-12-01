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

// isAuthorized returns whether the current user is authorized to use the Crimson API.
func isAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership").Err()
	}
	return is, err
}

// NewCrimsonServer returns a new Crimson RPC server.
func NewCrimsonServer() crimson.CrimsonServer {
	return &crimson.DecoratedCrimson{
		Prelude: CrimsonAuthPrelude,
		Service: &CrimsonService{},
	}
}

// CrimsonAuthPrelude ensures the user is authorized to use the Crimson API.
func CrimsonAuthPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch authorized, err := isAuthorized(c); {
	case err != nil:
		return c, InternalRPCError(c, err)
	case !authorized:
		return c, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	return c, nil
}

// CrimsonService handles Crimson RPCs.
type CrimsonService struct {
}

// GetDatacenters handles a request to retrieve datacenters.
func (s *CrimsonService) GetDatacenters(c context.Context, req *crimson.DatacentersRequest) (*crimson.DatacentersResponse, error) {
	datacenters, err := s.getDatacenters(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, InternalRPCError(c, err)
	}
	return &crimson.DatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// getDatacenters returns a list of datacenters in the database.
func (*CrimsonService) getDatacenters(c context.Context, names stringset.Set) ([]*crimson.Datacenter, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT name, description FROM datacenters")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	var datacenters []*crimson.Datacenter
	for rows.Next() {
		var name, description string
		if err = rows.Scan(&name, &description); err != nil {
			return nil, errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if names.Has(name) || names.Len() == 0 {
			datacenters = append(datacenters, &crimson.Datacenter{
				Name:        name,
				Description: description,
			})
		}
	}
	return datacenters, nil
}

// GetRacks handles a request to retrieve racks.
func (s *CrimsonService) GetRacks(c context.Context, req *crimson.RacksRequest) (*crimson.RacksResponse, error) {
	racks, err := s.getRacks(c, stringset.NewFromSlice(req.Names...), stringset.NewFromSlice(req.Datacenters...))
	if err != nil {
		return nil, InternalRPCError(c, err)
	}
	return &crimson.RacksResponse{
		Racks: racks,
	}, nil
}

// getRacks returns a list of racks in the database.
func (*CrimsonService) getRacks(c context.Context, names, datacenters stringset.Set) ([]*crimson.Rack, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT racks.name, racks.description, datacenters.name FROM racks, datacenters WHERE racks.datacenter_id = datacenters.id")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch racks").Err()
	}
	defer rows.Close()

	var racks []*crimson.Rack
	for rows.Next() {
		var name, description, datacenter string
		if err = rows.Scan(&name, &description, &datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch rack").Err()
		}
		if (names.Has(name) || names.Len() == 0) && (datacenters.Has(datacenter) || datacenters.Len() == 0) {
			racks = append(racks, &crimson.Rack{
				Name:        name,
				Description: description,
			})
		}
	}
	return racks, nil
}
