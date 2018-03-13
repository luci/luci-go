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
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

func init() {
	RegisterValidationVars(&validation.Rules)
}

// RegisterValidationVars registers placeholders (like ${appid}) that can be
// used in validation rules patterns.
//
// This function is called during init() time for default rule set.
func RegisterValidationVars(rules *validation.RuleSet) {
	rules.RegisterVar("appid", info.TrimmedAppID)
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
		switch s := mustFetchCachedSettings(cc); {
		case s.ConfigServiceHost == "":
			errStatus(cc, w, http.StatusInternalServerError, "ConfigServiceHost has not been defined in settings")
		case s.ConfigServiceEmail == "":
			errStatus(cc, w, http.StatusInternalServerError, "ConfigServiceEmail has not been defined in settings")
		case s.AdministratorsGroup == "":
			errStatus(cc, w, http.StatusInternalServerError, "AdministratorsGroup has not been defined in settings")
		default:
			if auth.CurrentIdentity(cc).Email() == s.ConfigServiceEmail {
				next(c)
				return
			}
			switch isAdmin, err := auth.IsMember(cc, s.AdministratorsGroup); {
			case err != nil:
				errStatus(cc, w, http.StatusInternalServerError, "Unable to authenticate")
			case !isAdmin:
				errStatus(cc, w, http.StatusForbidden, "Insufficient authority for validation")
			default:
				next(c)
			}
		}
	})
	validation.InstallHandlers(r, base, rules)
}

func errStatus(c context.Context, w http.ResponseWriter, status int, msg string) {
	if status >= http.StatusInternalServerError {
		logging.Errorf(c, msg)
	}
	w.WriteHeader(status)
	w.Write([]byte(msg))
}
