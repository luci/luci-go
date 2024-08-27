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
	"strings"

	"github.com/golang/protobuf/proto"
	// Store auth sessions in the datastore.
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/bisection/bqexporter"
	"go.chromium.org/luci/bisection/compilefailureanalysis/cancelanalysis"
	"go.chromium.org/luci/bisection/compilefailuredetection"
	"go.chromium.org/luci/bisection/culpritaction/revertculprit"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/metrics"
	pb "go.chromium.org/luci/bisection/proto/v1"
	bbpubsub "go.chromium.org/luci/bisection/pubsub"
	"go.chromium.org/luci/bisection/server"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/testfailuredetection"
	"go.chromium.org/luci/bisection/throttle"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"
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
		hosts.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		pubsub.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}
	luciAnalysisProject := ""
	flag.StringVar(
		&luciAnalysisProject, "luci-analysis-project", luciAnalysisProject, `the GCP project id of LUCI analysis.`,
	)
	luciAnalysisHost := ""
	flag.StringVar(
		&luciAnalysisHost, "luci-analysis-host", luciAnalysisHost, `the LUCI Analysis pRPC service hostname, e.g. analysis.api.luci.app.`,
	)

	uiRedirectURL := "luci-milo-dev.appspot.com/ui/bisection"
	flag.StringVar(
		&uiRedirectURL, "ui-redirect-url", uiRedirectURL, `the redirect url for the frontend.`,
	)

	luciserver.Main(nil, modules, func(srv *luciserver.Server) error {
		// Redirect the frontend to rpcexplorer.
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/rpcexplorer/", http.StatusFound)
		})
		srv.Routes.NotFound(nil, func(ctx *router.Context) {
			if strings.HasSuffix(ctx.Request.Host, ".appspot.com") {
				// Legacy LUCI Bisection URLs should redirect to MILO.
				url := fmt.Sprintf("https://%s%s", uiRedirectURL, ctx.Request.URL.Path)
				http.Redirect(ctx.Writer, ctx.Request, url, http.StatusFound)
			}
		})

		// Pubsub handler
		pubsub.RegisterJSONPBHandler("buildbucket", bbpubsub.BuildbucketPubSubHandler)

		pg := &LUCIAnalysisProject{
			DefaultProject: luciAnalysisProject,
		}

		// Installs gRPC service.
		pb.RegisterAnalysesServer(srv, &pb.DecoratedAnalyses{
			Service: &server.AnalysesServer{
				LUCIAnalysisHost: luciAnalysisHost,
			},
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
			// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
			s.HackFixFieldMasksForJSON = true
		})

		// GAE crons
		cron.RegisterHandler("update-config", config.UpdateProjects)
		cron.RegisterHandler("collect-global-metrics", metrics.CollectGlobalMetrics)
		cron.RegisterHandler("throttle-bisection", throttle.CronHandler)
		cron.RegisterHandler("export-test-analyses", bqexporter.ExportTestAnalyses)
		cron.RegisterHandler("ensure-views", bqexporter.EnsureViews)

		// Task queues
		compilefailuredetection.RegisterTaskClass()
		if err := revertculprit.RegisterTaskClass(srv, pg.Project); err != nil {
			return errors.Annotate(err, "register revert culprit").Err()
		}
		cancelanalysis.RegisterTaskClass()
		culpritverification.RegisterTaskClass()
		if err := testfailuredetection.RegisterTaskClass(srv, pg.Project); err != nil {
			return errors.Annotate(err, "register test failure detection").Err()
		}
		if err := bisection.RegisterTaskClass(srv, pg.Project); err != nil {
			return errors.Annotate(err, "register test failure bisector").Err()
		}

		return nil
	})
}

type LUCIAnalysisProject struct {
	DefaultProject string
}

// Project return LUCI Analysis GCP project given a LUCI Project.
// In normal cases, it will just check the app.yaml for the LUCI Analysis GCP project.
// However, for some project (eg. chrome) where we don't have dev data, we need to
// query from LUCI Analysis prod instead. In this case, we can set the GCP project here.
func (pg *LUCIAnalysisProject) Project(luciProject string) string {
	return pg.DefaultProject
}
