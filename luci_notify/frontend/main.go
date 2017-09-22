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

package frontend

import (
	"net/http"

	authServer "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/notify"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

func init() {
	r := router.New()
	standard.InstallHandlers(r)

	basemw := standard.Base().Extend(auth.Authenticate(authServer.CookieAuth))

	// Cron endpoint.
	r.GET("/internal/cron/update-config", basemw.Extend(gaemiddleware.RequireCron), config.UpdateHandler)

	// Pub/Sub endpoint.
	r.POST("/_ah/push-handlers/buildbucket", basemw, notify.BuildbucketPubSubHandler)

	http.Handle("/", r)
}
