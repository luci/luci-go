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

// Package server contains shared server initialisation logic for
// LUCI Analysis services.
package server

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/app"
	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	bugscron "go.chromium.org/luci/analysis/internal/bugs/cron"
	cpbq "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering/orchestrator"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/failureattributes"
	"go.chromium.org/luci/analysis/internal/metrics"
	"go.chromium.org/luci/analysis/internal/scopedauth"
	"go.chromium.org/luci/analysis/internal/services/bugupdater"
	"go.chromium.org/luci/analysis/internal/services/buildjoiner"
	"go.chromium.org/luci/analysis/internal/services/reclustering"
	"go.chromium.org/luci/analysis/internal/services/resultingester"
	"go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/views"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

// Main implements the common entrypoint for all LUCI Analysis GAE services.
// All LUCI Analysis GAE services have the code necessary to serve all pRPCs,
// crons and task queues. The only thing that is not shared is frontend
// handling, due to the fact this service requires other assets (javascript,
// files) to be deployed.
//
// Allowing all services to serve everything (except frontend) minimises
// the need to keep server code in sync with changes with dispatch.yaml.
// Moreover, dispatch.yaml changes are not deployed atomically
// with service changes, so this avoids transient traffic rejection during
// rollout of new LUCI Analysis versions that switch handling of endpoints
// between services.
func Main(init func(srv *luciserver.Server) error) {
	// Use the same modules for all LUCI Analysis services.
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(), // Required for auth sessions.
		gaeemulation.NewModuleFromFlags(),     // Needed by cfgmodule.
		secrets.NewModuleFromFlags(),          // Needed by encryptedcookies.
		spanmodule.NewModuleFromFlags(nil),
		scopedauth.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		buganizer.NewModuleFromFlags(),
	}
	luciserver.Main(nil, modules, func(srv *luciserver.Server) error {
		// Register pPRC servers.
		srv.ConfigurePRPC(func(s *prpc.Server) {
			s.AccessControl = prpc.AllowOriginAll
			// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
			s.HackFixFieldMasksForJSON = true
		})
		srv.RegisterUnaryServerInterceptors(span.SpannerDefaultsInterceptor())

		ac, err := analysis.NewClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return errors.Annotate(err, "creating analysis client").Err()
		}

		analysispb.RegisterClustersServer(srv, rpc.NewClustersServer(ac))
		analysispb.RegisterMetricsServer(srv, rpc.NewMetricsServer())
		analysispb.RegisterProjectsServer(srv, rpc.NewProjectsServer())
		analysispb.RegisterRulesServer(srv, rpc.NewRulesSever())
		analysispb.RegisterTestVariantsServer(srv, rpc.NewTestVariantsServer())
		analysispb.RegisterTestHistoryServer(srv, rpc.NewTestHistoryServer())
		analysispb.RegisterBuganizerTesterServer(srv, rpc.NewBuganizerTesterServer())
		analysispb.RegisterTestVariantBranchesServer(srv, rpc.NewTestVariantBranchesServer())

		// GAE crons.
		updateAnalysisAndBugsHandler := bugscron.NewHandler(srv.Options.CloudProject, srv.Options.Prod)
		cron.RegisterHandler("update-analysis-and-bugs", updateAnalysisAndBugsHandler.CronHandler)
		attributeFilteredTestRunsHandler := failureattributes.NewFilteredRunsAttributionHandler(srv.Options.CloudProject)
		cron.RegisterHandler("attribute-filtered-test-runs", attributeFilteredTestRunsHandler.CronHandler)
		cron.RegisterHandler("read-config", config.Update)
		cron.RegisterHandler("reclustering", orchestrator.CronHandler)
		cron.RegisterHandler("global-metrics", metrics.GlobalMetrics)
		cron.RegisterHandler("clear-rules-users", rules.ClearRulesUsers)
		cron.RegisterHandler("export-rules", func(ctx context.Context) error {
			return rules.ExportRulesCron(ctx, srv.Options.CloudProject)
		})
		cron.RegisterHandler("ensure-views", func(ctx context.Context) error {
			return views.CronHandler(ctx, srv.Options.CloudProject)
		})
		cron.RegisterHandler("merge-test-variant-branches", func(ctx context.Context) error {
			return cpbq.MergeTables(ctx, srv.Options.CloudProject)
		})

		// Pub/Sub subscription endpoints.
		srv.Routes.POST("/_ah/push-handlers/buildbucket", nil, app.BuildbucketPubSubHandler)
		srv.Routes.POST("/_ah/push-handlers/cvrun", nil, app.NewCVRunHandler().Handle)
		srv.Routes.POST("/_ah/push-handlers/invocation-finalized", nil, app.NewInvocationFinalizedHandler().Handle)

		// Register task queue tasks.
		if err := reclustering.RegisterTaskHandler(srv); err != nil {
			return errors.Annotate(err, "register reclustering").Err()
		}
		if err := resultingester.RegisterTaskHandler(srv); err != nil {
			return errors.Annotate(err, "register result ingester").Err()
		}
		if err := bugupdater.RegisterTaskHandler(srv); err != nil {
			return errors.Annotate(err, "register bug updater").Err()
		}
		buildjoiner.RegisterTaskClass()

		return init(srv)
	})
}
