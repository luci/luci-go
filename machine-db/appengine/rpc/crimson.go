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
)

// authPrelude ensures the user is authorized to use the Crimson API.
func authPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch authorized, err := isAuthorized(c); {
	case err != nil:
		return c, internalError(c, err)
	case !authorized:
		return c, status.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	return c, nil
}

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

// matches returns whether the given string matches the given set.
// An empty set matches all strings.
func matches(s string, set stringset.Set) bool {
	return set.Has(s) || set.Len() == 0
}

// stringSliceToInterfaceSlice converts the given []string to []interface.
func stringSliceToInterfaceSlice(s []string) []interface{} {
	if len(s) == 0 {
		return nil
	}
	slice := make([]interface{}, len(s))
	for i, j := range s {
		slice[i] = j
	}
	return slice
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
