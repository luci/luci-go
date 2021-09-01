// Copyright 2015 The LUCI Authors.
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

	"google.golang.org/appengine"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"
)

// UpdateConfigHandler is an HTTP handler that handles configuration update
// requests.
func UpdateConfigHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	// Needed to access the PubSub API
	c = appengine.WithContext(c, ctx.Request)
	projErr := common.UpdateProjects(c)
	if projErr != nil {
		if merr, ok := projErr.(errors.MultiError); ok {
			for _, ierr := range merr {
				logging.WithError(ierr).Errorf(c, "project update handler encountered error")
			}
		} else {
			logging.WithError(projErr).Errorf(c, "project update handler encountered error")
		}
	}
	settings, servErr := common.UpdateServiceConfig(c)
	if servErr != nil {
		logging.WithError(servErr).Errorf(c, "service update handler encountered error")
	} else {
		servErr = common.EnsurePubSubSubscribed(c, settings)
		if servErr != nil {
			logging.WithError(servErr).Errorf(
				c, "pubsub subscriber handler encountered error")
		}
	}
	if projErr != nil || servErr != nil {
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
