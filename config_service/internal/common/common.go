// Copyright 2023 The LUCI Authors.
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

// Package common holds the shared functionalities across config_service.
package common

import (
	"fmt"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
)

const (
	// ACLRegistryFilePath is the path of luci-config self config file where
	// it stores the service ACL configurations.
	ACLRegistryFilePath = "acl.cfg"

	// ProjRegistryFilePath is the path of luci-config self config file where it
	// stores a list of registered projects.
	ProjRegistryFilePath = "projects.cfg"

	// ProjMetadataFilePath is the path where project metadata config for each
	// LUCI Project.
	ProjMetadataFilePath = "project.cfg"

	// ServiceRegistryFilePath is the path of luci-config self config file where
	// it stores a list of registered services.
	ServiceRegistryFilePath = "services.cfg"

	// ImportConfigFilePath is the path of luci-config self config file where it
	// stores config about importing from external source.
	ImportConfigFilePath = "import.cfg"

	// SchemaConfigFilePath is the path of luci-config self config file where
	// it stores config to support schema redirection.
	SchemaConfigFilePath = "schemas.cfg"

	// GSProdCfgFolder is the folder name where it stores all production configs
	// in GCS bucket.
	GSProdCfgFolder = "configs"

	// GSValidationCfgFolder is the folder name where it stores all configs
	// for ad-hoc validation in GCS bucket.
	GSValidationCfgFolder = "validation"

	// ConfigMaxSize is the maximum size of the config LUCI Config supports.
	ConfigMaxSize = 200 * 1024 * 1024 // 200 MiB
)

// GitilesURL assembles a URL from the given gitiles location.
// Note: it doesn't validate the format of GitilesLocation.
func GitilesURL(loc *cfgcommonpb.GitilesLocation) string {
	if loc == nil || loc.Repo == "" {
		return ""
	}
	url := loc.Repo
	if loc.Ref != "" {
		url = fmt.Sprintf("%s/+/%s", url, loc.Ref)
	}
	if loc.Path != "" {
		url = fmt.Sprintf("%s/%s", url, loc.Path)
	}
	return url
}
