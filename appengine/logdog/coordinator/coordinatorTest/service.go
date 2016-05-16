// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Services is a testing stub for a coordinator.Services instance that allows
// the user to configure the various services that are returned.
type Services struct {
	// C, if not nil, will be used to get the return values for Config, overriding
	// local static members.
	C func() (*config.Config, error)

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
func (s *Services) Config(c context.Context) (*config.Config, error) {
	if s.C != nil {
		return s.C()
	}

	gcfg, err := config.Load(c)
	if err != nil {
		return nil, err
	}

	return gcfg, nil
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
