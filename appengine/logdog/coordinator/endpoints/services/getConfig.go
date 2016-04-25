// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

// GetConfig allows a service to retrieve the current service configuration
// parameters.
func (s *Server) GetConfig(c context.Context, req *google.Empty) (*logdog.GetConfigResponse, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	gcfg, _, err := coordinator.GetServices(c).Config(c)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to load configuration.")
		return nil, grpcutil.Internal
	}

	return &logdog.GetConfigResponse{
		ConfigServiceUrl: gcfg.ConfigServiceURL,
		ConfigSet:        gcfg.ConfigSet,
		ConfigPath:       gcfg.ConfigPath,
	}, nil
}
