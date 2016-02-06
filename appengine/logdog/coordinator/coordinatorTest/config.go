// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// UseConfig installs a Coordinator configuration into the current context.
func UseConfig(c context.Context, cc *svcconfig.Coordinator) context.Context {
	c = settings.Use(c, settings.New(&settings.MemoryStorage{}))
	gcfg := config.GlobalConfig{
		ConfigServiceURL: "https://example.com",
		ConfigSet:        "services/logdog-test",
		ConfigPath:       "coordinator-test.cfg",
	}
	if err := gcfg.Store(c, "test setup"); err != nil {
		panic(fmt.Errorf("failed to store test configuration: %v", err))
	}

	cmap := map[string]memory.ConfigSet{
		"services/logdog-test": map[string]string{},
	}
	if cc != nil {
		cfg := svcconfig.Config{
			Coordinator: cc,
		}
		cmap["services/logdog-test"]["coordinator-test.cfg"] = proto.MarshalTextString(&cfg)
	}
	return memory.Use(c, cmap)
}
