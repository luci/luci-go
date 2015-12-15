// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package configtest exposes functionality to help setup a LogDog
// configuration testing environment.
package configtest

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// GlobalConfig is the global configuration entry installed for the LogDog
// Coordinator.
var GlobalConfig = config.GlobalConfig{
	ConfigServiceURL: "https://example.com/luci-config",
	ConfigSet:        "services/luci-logdog-test",
	ConfigPath:       "coordinator.cfg",
}

// Install installs a testing configuration into the Context. This includes:
// - A settings.Storage MemoryStorage instance.
// - A memory.ConfigSet instance exposing the supplied test service
//   configuration.
func Install(c context.Context, cfg *config.Config) (context.Context, error) {
	buf := bytes.Buffer{}
	if err := proto.MarshalText(&buf, cfg); err != nil {
		return nil, err
	}

	c = settings.Use(c, settings.New(&settings.MemoryStorage{}))
	if err := GlobalConfig.Store(c, "testing"); err != nil {
		return nil, err
	}

	c = memory.Use(c, map[string]memory.ConfigSet{
		"services/luci-logdog-test": {
			"coordinator.cfg": buf.String(),
		},
	})
	return c, nil
}
