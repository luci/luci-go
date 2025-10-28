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

// Binary backend implements HTTP server that handles task queues and crons.
package main

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/appengine/impl/bootstrap"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"

	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

func main() {
	impl.Main(nil, func(srv *server.Server, svc *impl.Services) error {
		// Needed when using manual scaling.
		srv.Routes.GET("/_ah/start", nil, func(ctx *router.Context) {
			_, _ = ctx.Writer.Write([]byte("OK"))
		})
		srv.Routes.GET("/_ah/stop", nil, func(ctx *router.Context) {
			_, _ = ctx.Writer.Write([]byte("OK"))
		})

		// Periodically refresh the global service configs in the datastore.
		cron.RegisterHandler("import-config", func(ctx context.Context) error {
			importers := []func(context.Context) error{
				bootstrap.ImportConfig,
				prefixcfg.ImportConfig,
			}
			merr := make(errors.MultiError, len(importers))
			var wg sync.WaitGroup
			wg.Add(len(importers))
			for idx, importer := range importers {
				go func() {
					defer wg.Done()
					merr[idx] = importer(ctx)
				}()
			}
			wg.Wait()
			if merr.First() != nil {
				return merr.AsError()
			}
			return nil
		})

		// PubSub push handler processing messages produced by events.go.
		oidcMW := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)
		// bigquery-log-pubsub@ is a part of the PubSub Push subscription config.
		pusherID := identity.Identity(fmt.Sprintf("user:bigquery-log-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))
		srv.Routes.POST("/internal/pubsub/bigquery-log", oidcMW, func(ctx *router.Context) {
			if got := auth.CurrentIdentity(ctx.Request.Context()); got != pusherID {
				logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", pusherID, got)
				ctx.Writer.WriteHeader(403)
			} else {
				err := svc.EventLogger.HandlePubSubPush(ctx.Request.Context(), ctx.Request.Body)
				if err != nil {
					logging.Errorf(ctx.Request.Context(), "Failed to process the message: %s", err)
					ctx.Writer.WriteHeader(500)
				}
			}
		})

		return nil
	})
}
