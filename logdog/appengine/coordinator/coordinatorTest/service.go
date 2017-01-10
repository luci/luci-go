// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinatorTest

import (
	"github.com/luci/luci-go/logdog/api/config/svcconfig"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/appengine/coordinator/config"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	"golang.org/x/net/context"
)

// Services is a testing stub for a coordinator.Services instance that allows
// the user to configure the various services that are returned.
type Services struct {
	// C, if not nil, will be used to get the return values for Config, overriding
	// local static members.
	C func() (*config.Config, error)

	// PC, if not nil, will be used to get the return values for ProjectConfig,
	// overriding local static members.
	PC func() (*svcconfig.ProjectConfig, error)

	// Storage returns an intermediate storage instance for use by this service.
	//
	// The caller must close the returned instance if successful.
	//
	// By default, this will return a *BigTableStorage instance bound to the
	// Environment's BigTable instance if the stream is not archived, and an
	// *ArchivalStorage instance bound to this Environment's GSClient instance
	// if the stream is archived.
	ST func(*coordinator.LogStreamState) (coordinator.Storage, error)

	// ArchivalPublisher returns an ArchivalPublisher instance.
	AP func() (coordinator.ArchivalPublisher, error)
}

var _ coordinator.Services = (*Services)(nil)

// Config implements coordinator.Services.
func (s *Services) Config(c context.Context) (*config.Config, error) {
	if s.C != nil {
		return s.C()
	}
	return config.Load(c)
}

// ProjectConfig implements coordinator.Services.
func (s *Services) ProjectConfig(c context.Context, project cfgtypes.ProjectName) (*svcconfig.ProjectConfig, error) {
	if s.PC != nil {
		return s.PC()
	}
	return config.ProjectConfig(c, project)
}

// StorageForStream implements coordinator.Services.
func (s *Services) StorageForStream(c context.Context, lst *coordinator.LogStreamState) (coordinator.Storage, error) {
	if s.ST != nil {
		return s.ST(lst)
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
