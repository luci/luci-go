// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// GetConfigResponse is the response structure for the user
// "GetConfig" endpoint.
type GetConfigResponse struct {
	// ConfigServiceURL is the API URL of the base "luci-config" service. If
	// empty, the default service URL will be used.
	ConfigServiceURL string `json:"configService"`

	// ConfigSet is the name of the configuration set to load from.
	ConfigSet string `json:"configSet"`
	// ConfigPath is the path of the text-serialized configuration protobuf.
	ConfigPath string `json:"configPath,omitempty"`
}

// GetConfig allows a service to retrieve the current service configuration
// parameters.
func (s *Service) GetConfig(c context.Context) (*GetConfigResponse, error) {
	c, err := s.Use(c, MethodInfoMap["GetConfig"])
	if err != nil {
		return nil, err
	}
	if err := Auth(c); err != nil {
		return nil, err
	}

	cfg, err := config.LoadGlobalConfig(c)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to load configuration.")
		return nil, endpoints.NewInternalServerError("failed to get config")
	}

	return &GetConfigResponse{
		ConfigServiceURL: cfg.ConfigServiceURL,
		ConfigSet:        cfg.ConfigSet,
		ConfigPath:       cfg.ConfigPath,
	}, nil
}
