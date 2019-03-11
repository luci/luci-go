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
	"net/http"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/router"
)

func init() {
	RegisterValidationVars(&validation.Rules)
}

// RegisterValidationVars registers placeholders that can be used in validation
// rules patterns.
//
// Registers:
//    ${appid} - expands into a GAE app ID of the running service.
//    ${config_service_appid} - expands into a GAE app ID of a LUCI Config
//        service that the running service is using (or empty string if
//        unconfigured).
//
// This function is called during init() time for default rule set.
func RegisterValidationVars(rules *validation.RuleSet) {
	rules.RegisterVar("appid", func(c context.Context) (string, error) {
		return info.TrimmedAppID(c), nil
	})
	rules.RegisterVar("config_service_appid", GetConfigServiceAppID)
}

// InstallValidationHandlers installs handlers for config validation.
//
// It ensures that caller is either the config service itself or a member of a
// trusted group, both of which are configurable in the appengine app settings.
// It requires that the hostname, the email of config service and the name of
// the trusted group have been defined in the appengine app settings page before
// the installed endpoints are called.
func InstallValidationHandlers(r *router.Router, base router.MiddlewareChain, rules *validation.RuleSet) {
	a := auth.Authenticator{
		Methods: []auth.Method{
			&server.OAuth2Method{Scopes: []string{server.EmailScope}},
		},
	}
	base = base.Extend(a.GetMiddleware(), func(c *router.Context, next router.Handler) {
		cc, w := c.Context, c.Writer
		switch yep, err := isAuthorizedCall(cc, mustFetchCachedSettings(cc)); {
		case err != nil:
			errStatus(cc, w, err, http.StatusInternalServerError, "Unable to perform authorization")
		case !yep:
			errStatus(cc, w, nil, http.StatusForbidden, "Insufficient authority for validation")
		default:
			next(c)
		}
	})
	validation.InstallHandlers(r, base, rules)
}

func errStatus(c context.Context, w http.ResponseWriter, err error, status int, msg string) {
	if status >= http.StatusInternalServerError {
		if err != nil {
			c = logging.SetError(c, err)
		}
		logging.Errorf(c, "%s", msg)
	}
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// isAuthorizedCall returns true if the current caller is allowed to call the
// config validation endpoints.
//
// This is either the service account of the config service, or someone from
// an admin group.
func isAuthorizedCall(c context.Context, s *Settings) (bool, error) {
	// Someone from an admin group (if it is configured)? This is useful locally
	// during development.
	if s.AdministratorsGroup != "" {
		switch yep, err := auth.IsMember(c, s.AdministratorsGroup); {
		case err != nil:
			return false, err
		case yep:
			return true, nil
		}
	}

	// The config server itself (if it is configured)? May be empty when
	// running stuff locally.
	if s.ConfigServiceHost != "" {
		info, err := signing.FetchServiceInfoFromLUCIService(c, "https://"+s.ConfigServiceHost)
		if err != nil {
			return false, err
		}
		caller := auth.CurrentIdentity(c)
		if caller.Kind() == identity.User && caller.Value() == info.ServiceAccountName {
			return true, nil
		}
	}

	// A total stranger.
	return false, nil
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
