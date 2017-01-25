// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
