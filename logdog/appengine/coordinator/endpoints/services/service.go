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

package services

import (
	"github.com/golang/protobuf/proto"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/common/cfgtypes"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"

	"golang.org/x/net/context"
)

// server is a service supporting privileged support services.
//
// This endpoint is restricted to LogDog support service accounts.
type server struct{}

// New creates a new authenticating ServicesServer instance.
func New() logdog.ServicesServer {
	return &logdog.DecoratedServices{
		Service: &server{},
		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			// Only service users may access this endpoint.
			if err := coordinator.IsServiceUser(c); err != nil {
				log.WithError(err).Errorf(c, "Failed to authenticate user as a service.")

				if !coordinator.IsMembershipError(err) {
					// Not a membership error. Something went wrong on the server's end.
					return nil, grpcutil.Internal
				}
				return nil, grpcutil.PermissionDenied
			}

			return maybeEnterProjectNamespace(c, req)
		},
	}
}

// maybeEnterProjectNamespace enters a datastore namespace based on the request
// message type.
func maybeEnterProjectNamespace(c context.Context, req proto.Message) (context.Context, error) {
	if pbm, ok := req.(endpoints.ProjectBoundMessage); ok {
		project := cfgtypes.ProjectName(pbm.GetMessageProject())
		log.Fields{
			"project": project,
		}.Debugf(c, "Request is entering project namespace.")
		if err := coordinator.WithProjectNamespace(&c, project, coordinator.NamespaceAccessNoAuth); err != nil {
			return c, err
		}
	}
	return c, nil
}
