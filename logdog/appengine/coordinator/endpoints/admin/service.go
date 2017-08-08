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

package admin

import (
	"github.com/golang/protobuf/proto"
	"go.chromium.org/gae/service/info"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/admin/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
)

// server is the service implementation for the administrator endpoint.
type server struct{}

// New instantiates a new AdminServer instance.
func New() logdog.AdminServer {
	return &logdog.DecoratedAdmin{
		Service: &server{},
		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			if err := coordinator.IsAdminUser(c); err != nil {
				log.WithError(err).Warningf(c, "User is not an administrator.")

				// If we're on development server, any user can access this endpoint.
				if info.IsDevAppServer(c) {
					log.Infof(c, "On development server, allowing admin access.")
					return c, nil
				}

				u := auth.CurrentUser(c)
				if u == nil || !u.Superuser {
					return nil, grpcutil.PermissionDenied
				}

				log.Fields{
					"email":    u.Email,
					"clientID": u.ClientID,
					"name":     u.Name,
				}.Infof(c, "User is an AppEngine superuser. Granting access.")
			}

			return c, nil
		},
	}
}
