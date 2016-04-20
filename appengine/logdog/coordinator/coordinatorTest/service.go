// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"errors"

	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Services is a testing stub for a coordinator.Services instance that allows
// the user to configure the various services that are returned.
type Services struct {
	// GlobalConfig is the global configuration to return from Config.
	GlobalConfig *config.GlobalConfig
	// ServiceConfig is the service configuration to return from Config.
	ServiceConfig *svcconfig.Config

	// C, if not nil, will be used to get the return values for Config, overriding
	// local static members.
	C func() (*config.GlobalConfig, *svcconfig.Config, error)

	// Storage returns an intermediate storage instance for use by this service.
	//
	// The caller must close the returned instance if successful.
	IS func() (storage.Storage, error)

	// GSClient instantiates a Google Storage client.
	GS func() (gs.Client, error)

	// ArchivalPublisher returns an ArchivalPublisher instance.
	AP func() (coordinator.ArchivalPublisher, error)
}

var _ coordinator.Services = (*Services)(nil)

// Config implements coordinator.Services.
func (s *Services) Config(context.Context) (*config.GlobalConfig, *svcconfig.Config, error) {
	if s.C != nil {
		return s.C()
	}
	if s.GlobalConfig == nil || s.ServiceConfig == nil {
		return nil, nil, errors.New("not configured")
	}
	return s.GlobalConfig, s.ServiceConfig, nil
}

// IntermediateStorage implements coordinator.Services.
func (s *Services) IntermediateStorage(context.Context) (storage.Storage, error) {
	if s.IS != nil {
		return s.IS()
	}
	panic("not implemented")
}

// GSClient implements coordinator.Services.
func (s *Services) GSClient(context.Context) (gs.Client, error) {
	if s.GS != nil {
		return s.GS()
	}
	panic("not implemented")
}

// ArchivalPublisher implements coordinator.Services.
func (s *Services) ArchivalPublisher(context.Context) (coordinator.ArchivalPublisher, error) {
	if s.AP != nil {
		return s.AP()
	}
	panic("not implemented")
}

// InitConfig loads default testing GlobalConfig and ServiceConfig values.
func (s *Services) InitConfig() {
	s.GlobalConfig = &config.GlobalConfig{
		ConfigServiceURL: "https://example.com",
		ConfigSet:        "services/logdog-test",
		ConfigPath:       "coordinator-test.cfg",
	}

	s.ServiceConfig = &svcconfig.Config{
		Coordinator: &svcconfig.Coordinator{},
	}
}
