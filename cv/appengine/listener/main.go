// Copyright 2022 The LUCI Authors.
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

// Package main is the main entry point for the app.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	gerritsub "go.chromium.org/luci/cv/internal/gerrit/subscriber"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	var gerritPubsubServiceAccount string
	flag.StringVar(&gerritPubsubServiceAccount, "gerrit-pubsub-push-account", "", "the service account of pubsub pusher")

	server.Main(nil, modules, func(srv *server.Server) error {
		clUpdater := changelist.NewUpdater(&tq.Default, nil)
		oidcMW := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)

		gerritSubscriber := &gerritsub.GerritSubscriber{CLUpdater: clUpdater}
		if gerritPubsubServiceAccount == "" {
			return errors.New("gerrit pubsub pusher account must be specified")
		}
		gerritPubSubIdentity, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, gerritPubsubServiceAccount))
		if err != nil {
			return errors.Fmt("failed to create identity for %s: %w", gerritPubsubServiceAccount, err)
		}
		srv.Routes.POST("/pubsub-handlers/gerrit/:host/:format", oidcMW, makePubsubHandler(func(ctx *router.Context, payload common.PubSubMessagePayload) error {
			hostName := fmt.Sprintf("%s-review.googlesource.com", ctx.Params.ByName("host"))
			return gerritSubscriber.ProcessPubSubMessage(ctx.Request.Context(), payload,
				hostName, ctx.Params.ByName("format"))
		}, gerritPubSubIdentity))

		return nil
	})
}

func makePubsubHandler(handleFn func(ctx *router.Context, payload common.PubSubMessagePayload) error, pubsubPusherIdentity identity.Identity) router.Handler {
	return func(ctx *router.Context) {
		if got := auth.CurrentIdentity(ctx.Request.Context()); got != pubsubPusherIdentity {
			logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", pubsubPusherIdentity, got)
			http.Error(ctx.Writer, "Forbidden", http.StatusForbidden)
			return
		}
		var payload common.PubSubMessagePayload
		if err := json.NewDecoder(ctx.Request.Body).Decode(&payload); err != nil {
			logging.Errorf(ctx.Request.Context(), "Error while decoding Pub/Sub message: %v", err)
			http.Error(ctx.Writer, "Could not decode Pub/Sub message", http.StatusBadRequest)
			return
		}
		if err := handleFn(ctx, payload); err != nil {
			logging.Errorf(ctx.Request.Context(), "Error while processing Pub/Sub message %q: %v", payload.Message.MessageID, err)
			http.Error(ctx.Writer, "Could not process Pub/Sub message", http.StatusInternalServerError)
			return
		}
		ctx.Writer.WriteHeader(http.StatusOK)
		fmt.Fprintln(ctx.Writer, "Pub/Sub message processed successfully")
	}
}
