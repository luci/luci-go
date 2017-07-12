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
	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/services/v1"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/luci_config/appengine/gaeconfig"

	"golang.org/x/net/context"
)

// GetConfig allows a service to retrieve the current service configuration
// parameters.
func (s *server) GetConfig(c context.Context, req *empty.Empty) (*logdog.GetConfigResponse, error) {
	gcfg, err := coordinator.GetServices(c).Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load configuration.")
		return nil, grpcutil.Internal
	}

	// Load our config service host from settings.
	settings, err := gaeconfig.FetchCachedSettings(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load settings.")
		return nil, grpcutil.Internal
	}

	return &logdog.GetConfigResponse{
		ConfigServiceHost: settings.ConfigServiceHost,
		ConfigSet:         string(gcfg.ConfigSet),
		ServiceConfigPath: gcfg.ServiceConfigPath,

		// TODO(dnj): Deprecate this field once everything has switched over to
		// using host.
		ConfigServiceUrl: gcfg.ConfigServiceURL.String(),
	}, nil
}
