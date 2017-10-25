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

package module

import (
	"go.chromium.org/luci/grpc/grpcutil"
	logsPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// dummyLogsService is a logsPb.Service implementation that always returns
// a "not implemented" error.
var dummyLogsService = &logsPb.DecoratedLogs{
	Service: nil,
	Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
		return nil, grpcutil.Errf(codes.FailedPrecondition,
			"Logs traffic cannot be handled by the default service. It must be routed to the logs "+
				"service. This error indicates that the routing is not working as intended. Please report "+
				"this error to the maintainers.")
	},
}
