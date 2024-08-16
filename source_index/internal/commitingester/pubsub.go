// Copyright 2024 The LUCI Authors.
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

package commitingester

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
)

// RegisterPubSubHandlers registers the pubsub handlers for Source Index's
// commit ingestion.
func RegisterPubSubHandlers(srv *server.Server) error {
	pusherID := identity.Identity(fmt.Sprintf("user:gitiles-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))

	// A middleware that restrict the endpoint to the pubsub service account.
	pubsubMW := router.NewMiddlewareChain(
		auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
			AudienceCheck: openid.AudienceMatchesHost,
		}),
		func(ctx *router.Context, next router.Handler) {
			if got := auth.CurrentIdentity(ctx.Request.Context()); got != pusherID {
				logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", pusherID, got)
				ctx.Writer.WriteHeader(http.StatusForbidden)
				return
			}
			next(ctx)
		},
	)

	// PubSub subscription endpoints.
	srv.Routes.POST("/push-handlers/gitiles/:gitiles_host", pubsubMW, pubSubHandler)

	return nil
}

func pubSubHandler(c *router.Context) {
	// TODO(b/356027716): implement commit ingestion.
	c.Writer.WriteHeader(http.StatusOK)
}
