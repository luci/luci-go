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

// Package main implements the App Engine based HTTP server to handle request
// to LUCI Bisection
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"go.chromium.org/luci/bisection/compilefailureanalysis/cancelanalysis"
	"go.chromium.org/luci/bisection/compilefailuredetection"
	"go.chromium.org/luci/bisection/culpritaction/revertculprit"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/frontend/handlers"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/metrics"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/pubsub"
	"go.chromium.org/luci/bisection/server"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/testfailuredetection"
	"go.chromium.org/luci/bisection/throttle"
	"go.chromium.org/luci/grpc/prpc"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/server/cfgmodule"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/tq"

	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"

	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ACCESS_GROUP         = "luci-bisection-access"
	ACCESS_GROUP_FOR_BOT = "luci-bisection-bot-access"
)

func checkAPIAccess(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch yes, err := auth.IsMember(ctx, ACCESS_GROUP); {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error when checking group membership")
	case !yes:
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have access to method %s of GoFindit", auth.CurrentIdentity(ctx), methodName)
	default:
		return ctx, nil
	}
}

func checkBotAPIAccess(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch yes, err := auth.IsMember(ctx, ACCESS_GROUP_FOR_BOT); {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error when checking group membership for bot")
	case !yes:
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have access to method %s of GoFindit", auth.CurrentIdentity(ctx), methodName)
	default:
		return ctx, nil
	}
}

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(), // Required for auth sessions.
		secrets.NewModuleFromFlags(),          // Needed by encryptedcookies.
		tq.NewModuleFromFlags(),
	}
	luciAnalysisProject := ""
	flag.StringVar(
		&luciAnalysisProject, "luci-analysis-project", luciAnalysisProject, `the GCP project id of LUCI analysis.`,
	)
	uiRedirectURL := "luci-milo-dev.appspot.com/ui/bisection"
	flag.StringVar(
		&uiRedirectURL, "ui-redirect-url", uiRedirectURL, `the redirect url for the frontend.`,
	)

	luciserver.Main(nil, modules, func(srv *luciserver.Server) error {
		// Redirect the frontend to Milo.
		srv.Routes.NotFound(nil, handlers.Redirect(uiRedirectURL))

		// Pubsub handler
		pubsubMwc := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)
		pusherID := identity.Identity(fmt.Sprintf("user:buildbucket-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))

		srv.Routes.POST("/_ah/push-handlers/buildbucket", pubsubMwc, func(ctx *router.Context) {
			if got := auth.CurrentIdentity(ctx.Request.Context()); got != pusherID {
				logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", pusherID, got)
				ctx.Writer.WriteHeader(http.StatusForbidden)
			} else {
				pubsub.BuildbucketPubSubHandler(ctx)
			}
		})

		// Installs gRPC service.
		pb.RegisterAnalysesServer(srv, &pb.DecoratedAnalyses{
			Service: &server.AnalysesServer{},
			Prelude: checkAPIAccess,
		})

		// Installs gRPC service to communicate with recipes
		pb.RegisterBotUpdatesServer(srv, &pb.DecoratedBotUpdates{
			Service: &server.BotUpdatesServer{},
			Prelude: checkBotAPIAccess,
		})

		// Register pPRC servers.
		srv.ConfigurePRPC(func(s *prpc.Server) {
			// Allow cross-origin calls.
			s.AccessControl = prpc.AllowOriginAll
		})

		// GAE crons
		cron.RegisterHandler("update-config", config.Update)
		cron.RegisterHandler("collect-global-metrics", metrics.CollectGlobalMetrics)
		cron.RegisterHandler("throttle-bisection", throttle.CronHandler)

		// Task queues
		compilefailuredetection.RegisterTaskClass()
		revertculprit.RegisterTaskClass()
		cancelanalysis.RegisterTaskClass()
		culpritverification.RegisterTaskClass()
		if err := testfailuredetection.RegisterTaskClass(srv, luciAnalysisProject); err != nil {
			return errors.Annotate(err, "register test failure detection").Err()
		}
		if err := bisection.RegisterTaskClass(srv, luciAnalysisProject); err != nil {
			return errors.Annotate(err, "register test failure bisector").Err()
		}

		return nil
	})
}
