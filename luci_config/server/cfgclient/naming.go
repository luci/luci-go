// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cfgclient

import (
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/luci_config/common/cfgtypes"

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
