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
