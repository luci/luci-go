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

package main

import (
	"context"

	"go.chromium.org/luci/grpc/grpcutil"
	logsPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	registrationPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
)

// dummyLogsService is a logsPb.Service implementation that always returns
// a "codes.FailedPrecondition" error.
var dummyLogsService = &logsPb.DecoratedLogs{
	Prelude: dummyServicePrelude,
}

// dummyRegistrationService is a registrationPb.Service implementation that
// always returns a "codes.FailedPrecondition" error.
var dummyRegistrationService = &registrationPb.DecoratedRegistration{
	Prelude: dummyServicePrelude,
}

// dummyServicesService is a servicesPb.Service implementation that always
// returns a "codes.FailedPrecondition" error.
var dummyServicesService = &servicesPb.DecoratedServices{
	Prelude: dummyServicePrelude,
}

// dummyServicePrelude is a service decorator prelude that always returns
// "codes.FailedPrecondition".
// This is implemented this way so that the decorated service can be made
// discoverable without actually hosting an implementation of that service.
// The actual implementation is hosted in a separate GAE service, and RPCs
// directed directed at this dummy service should be automatically routed to
// the implementing GAE service through "dispatch.yaml".
func dummyServicePrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	return nil, grpcutil.Errf(codes.FailedPrecondition,
		"This pRPC endpoint cannot be handled by the default service. It should have been routed to the "+
			"appropriate service via dispatch. This error indicates that the routing is not working as intended. "+
			"Please report this error to the maintainers.")
}
