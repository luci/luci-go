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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// DeleteHost handles a request to delete an existing host.
func (*Service) DeleteHost(c context.Context, req *crimson.DeleteHostRequest) (*empty.Empty, error) {
	if err := deleteHost(c, req.Name, req.Vlan); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// deleteHost deletes an existing host from the database.
func deleteHost(c context.Context, name string, vlan int64) error {
	switch {
	case name == "":
		return status.Error(codes.InvalidArgument, "hostname is required and must be non-empty")
	case vlan < 1:
		return status.Error(codes.InvalidArgument, "VLAN is required and must be positive")
	}

	db := database.Get(c)
	res, err := db.ExecContext(c, `
		DELETE FROM hostnames WHERE name = ? AND vlan_id = ?
	`, name, vlan)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_ROW_IS_REFERENCED_2 && strings.Contains(e.Message, "`physical_host_id`"):
			// e.g. "Error 1452: Cannot add or update a child row: a foreign key constraint fails (FOREIGN KEY (`physical_host_id`) REFERENCES `physical_hosts` (`id`))".
			return status.Errorf(codes.FailedPrecondition, "delete entities referencing this host first")
		}
		return errors.Annotate(err, "failed to delete host").Err()
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return errors.Annotate(err, "failed to fetch affected rows").Err()
	case rows == 0:
		return status.Errorf(codes.NotFound, "unknown host %q for VLAN %d", name, vlan)
	case rows == 1:
		return nil
	default:
		// Shouldn't happen because name is unique in the database.
		return errors.Reason("unexpected number of affected rows %d", rows).Err()
	}
}
