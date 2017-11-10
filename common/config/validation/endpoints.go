// Copyright 2017 The LUCI Authors.
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

package validation

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	luciConfigName = "luci-config@appspot.gserviceaccount.com"
)

var errStatus = func(c context.Context, w http.ResponseWriter, status int, msg string) {
	logging.Errorf(c, "Status %d msg %s", status, msg)
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// Checks whether the requester is luci-config or not
func checkAuth(c *router.Context, next router.Handler) {
	if auth.CurrentIdentity(c.Context).Email() == luciConfigName {
		next(c)
	} else {
		errStatus(c.Context, c.Writer, http.StatusForbidden, "Access denied")
	}
}

func base() router.MiddlewareChain {
	a := auth.Authenticator{
		Methods: []auth.Method{
			&server.OAuth2Method{Scopes: []string{server.EmailScope}},
			&server.InboundAppIDAuthMethod{},
		},
	}
	return standard.Base().Extend(a.GetMiddleware())
}

// InitializeValidationEndpoints sets up the config validation endpoints.
func InitializeValidationEndpoints(validator Validator, metadata Metadata) {
	r := router.New()
	authmw := base().Extend(checkAuth)

	standard.InstallHandlers(r)

	r.GET("/api/config/v1/metadata", authmw, metadata.GetMetadataHandler)
	r.POST("/api/config/v1/validate", authmw, validator.ValidationHandler)

	http.DefaultServeMux.Handle("/api/", r)
}
