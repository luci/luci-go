// Copyright 2018 The LUCI Authors.
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

package gaeconfig

import (
	"context"

	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/config/vars"
)

func init() {
	RegisterVars(&vars.Vars)
}

// RegisterVars registers placeholders that can be used in config set names.
//
// Registers:
//
//	${appid} - expands into a GAE app ID of the running service.
//	${config_service_appid} - expands into a GAE app ID of a LUCI Config
//	    service that the running service is using (or empty string if
//	    unconfigured).
//
// This function is called during init() with the default var set.
func RegisterVars(vars *vars.VarSet) {
	vars.Register("appid", func(c context.Context) (string, error) {
		return info.TrimmedAppID(c), nil
	})
	vars.Register("config_service_appid", GetConfigServiceAppID)
}

// GetConfigServiceAppID looks up the app ID of the LUCI Config service, as set
// in the app's settings.
//
// Returns an empty string if the LUCI Config integration is not configured for
// the app.
func GetConfigServiceAppID(c context.Context) (string, error) {
	s, err := FetchCachedSettings(c)
	switch {
	case err != nil:
		return "", err
	case s.ConfigServiceHost == "":
		return "", nil
	}
	info, err := signing.FetchServiceInfoFromLUCIService(c, "https://"+s.ConfigServiceHost)
	if err != nil {
		return "", err
	}
	return info.AppID, nil
}
