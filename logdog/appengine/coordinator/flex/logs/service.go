// Copyright 2015 The LUCI Authors.
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

package logs

import (
	"context"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/common/types"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
)

// Server is the user-facing log access and query endpoint service.
type server struct {
	// resultLimit is the maximum number of query results to return in a
	// single query. If zero, the default will be used.
	//
	// This is provided for testing purposes.
	resultLimit int
}

// New creates a new authenticating LogsServer instance.
func New() logdog.LogsServer {
	return newService(&server{})
}

func newService(svr *server) logdog.LogsServer {
	return &logdog.DecoratedLogs{
		Service: svr,
		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			// Enter a datastore namespace based on the message type.
			//
			// We use a type switch here because this is a shared decorator. All user
			// messages must implement ProjectBoundMessage.
			pbm, ok := req.(endpoints.ProjectBoundMessage)
			if ok {
				// Enter the requested project namespace. This validates that the
				// current user has READ access.
				project := types.ProjectName(pbm.GetMessageProject())
				if project == "" {
					return nil, grpcutil.Errf(codes.InvalidArgument, "project is required")
				}

				log.Fields{
					"project": project,
				}.Debugf(c, "User is accessing project.")
				if err := coordinator.WithProjectNamespace(&c, project, coordinator.NamespaceAccessREAD); err != nil {
					return nil, getGRPCError(err)
				}
			}

			return c, nil
		},
	}
}

func getGRPCError(err error) error {
	switch {
	case err == nil:
		return nil

	case grpcutil.Code(err) != codes.Unknown:
		// If this is already a gRPC error, return it directly.
		return err

	default:
		// Generic empty internal error.
		return grpcutil.Internal
	}
}

func (s *server) limit(v int, d int) int {
	if s.resultLimit > 0 {
		d = s.resultLimit
	}
	if v <= 0 || v > d {
		return d
	}
	return v
}
