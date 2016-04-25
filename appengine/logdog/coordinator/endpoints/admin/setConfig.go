// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// SetConfig loads the supplied configuration into a config.GlobalConfig
// instance.
func (s *Server) SetConfig(c context.Context, req *logdog.SetConfigRequest) (*google.Empty, error) {
	if err := s.Auth(c); err != nil {
		return nil, err
	}

	gcfg := config.GlobalConfig{
		ConfigServiceURL:           req.ConfigServiceUrl,
		ConfigSet:                  req.ConfigSet,
		ConfigPath:                 req.ConfigPath,
		BigTableServiceAccountJSON: req.StorageServiceAccountJson,
	}
	if err := gcfg.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "New configuration did not validate.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "config did not validate: %v", err)
	}

	if err := gcfg.Store(c, "setConfig endpoint"); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to store new configuration.")
		return nil, grpcutil.Internal
	}
	return &google.Empty{}, nil
}
