// Copyright 2016 The LUCI Authors.
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

package registration

import (
	"context"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/grpcutil"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
)

// server is a service supporting log stream registration.
type server struct{}

// New creates a new authenticating ServicesServer instance.
func New() logdog.RegistrationServer {
	return &logdog.DecoratedRegistration{
		Service: &server{},
		Prelude: func(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
			// Enter a datastore namespace based on the message type. All RPC messages
			// in RegistrationServer must implement ProjectBoundMessage. We panic if
			// they don't.
			project := req.(endpoints.ProjectBoundMessage).GetMessageProject()
			if project == "" {
				return nil, status.Error(codes.InvalidArgument, "project is required")
			}
			if err := coordinator.WithProjectNamespace(&ctx, project); err != nil {
				return nil, grpcutil.GRPCifyAndLogErr(ctx, err)
			}
			return ctx, nil
		},
	}
}
