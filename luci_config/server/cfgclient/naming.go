// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cfgclient

import (
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	"golang.org/x/net/context"
)

// ProjectConfigPath is the path of a project's project-wide configuration file.
const ProjectConfigPath = "project.cfg"

// CurrentServiceName returns the current service name, as used to identify it
// in configurations. This is based on the current App ID.
func CurrentServiceName(c context.Context) string { return info.TrimmedAppID(c) }

// CurrentServiceConfigSet returns the config set for the current AppEngine
// service, based on its current service name.
func CurrentServiceConfigSet(c context.Context) cfgtypes.ConfigSet {
	return cfgtypes.ServiceConfigSet(CurrentServiceName(c))
}
