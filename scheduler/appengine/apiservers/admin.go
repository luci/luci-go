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

package apiservers

import (
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	schedulerpb "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/server/auth"
)

// AdminServerWithACL returns AdminServer implementation that checks all callers
// are in the given administrator group.
func AdminServerWithACL(e engine.EngineInternal, c catalog.Catalog, adminGroup string) internal.AdminServer {
	return &internal.DecoratedAdmin{
		Service: &adminServer{
			Engine:  e,
			Catalog: c,
		},

		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			caller := auth.CurrentIdentity(c)
			logging.Warningf(c, "Admin call %q by %q", methodName, caller)
			switch yes, err := auth.IsMember(c, adminGroup); {
			case err != nil:
				return nil, status.Errorf(codes.Internal, "failed to check ACL")
			case !yes:
				return nil, status.Errorf(codes.PermissionDenied, "not an administrator")
			default:
				return c, nil
			}
		},

		Postlude: func(c context.Context, methodName string, rsp proto.Message, err error) error {
			return grpcutil.GRPCifyAndLogErr(c, err)
		},
	}
}

// adminServer implements internal.admin.Admin API without ACL check.
//
// It also returns regular errors, NOT gRPC errors. AdminServerWithACL takes
// care of authorization and conversion of errors to grpc ones.
type adminServer struct {
	Engine  engine.EngineInternal
	Catalog catalog.Catalog
}

// GetInternalJobState implements the corresponding RPC method.
func (s *adminServer) GetInternalJobState(c context.Context, r *schedulerpb.JobRef) (resp *internal.InternalJobState, err error) {
	// TODO(vadimsh): Implement.
	return nil, nil
}
