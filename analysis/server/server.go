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
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/secrets"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/app"
	"go.chromium.org/luci/analysis/internal/admin"
	adminpb "go.chromium.org/luci/analysis/internal/admin/proto"
	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	bugscron "go.chromium.org/luci/analysis/internal/bugs/cron"
	"go.chromium.org/luci/analysis/internal/changepoints"
	cpbq "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	bqupdator "go.chromium.org/luci/analysis/internal/changepoints/bqupdater"
	"go.chromium.org/luci/analysis/internal/changepoints/groupscheduler"
	"go.chromium.org/luci/analysis/internal/changepoints/sorbet"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering/orchestrator"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/failureattributes"
	"go.chromium.org/luci/analysis/internal/hosts"
	"go.chromium.org/luci/analysis/internal/metrics"
	"go.chromium.org/luci/analysis/internal/scopedauth"
	"go.chromium.org/luci/analysis/internal/services/backfill"
	"go.chromium.org/luci/analysis/internal/services/bugupdater"
	"go.chromium.org/luci/analysis/internal/services/buildjoiner"
	"go.chromium.org/luci/analysis/internal/services/changepointgrouper"
	"go.chromium.org/luci/analysis/internal/services/reclustering"
	"go.chromium.org/luci/analysis/internal/services/resultingester"
	"go.chromium.org/luci/analysis/internal/services/verdictingester"
	"go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/internal/views"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

// Main implements the common entrypoint for all LUCI Analysis GAE services.
//
// Note, if changing responsibiltiy between services, please be aware
// that dispatch.yaml changes are not deployed atomically with service
// changes.
func Main(init func(srv *luciserver.Server) error) {
	// Use the same modules for all LUCI Analysis services.
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(), // Required for auth sessions.
		gaeemulation.NewModuleFromFlags(),     // Needed by cfgmodule.
		hosts.NewModuleFromFlags(),
		pubsub.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(), // Needed by encryptedcookies.
		spanmodule.NewModuleFromFlags(nil),
		scopedauth.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		buganizer.NewModuleFromFlags(),
	}
	luciserver.Main(nil, modules, init)
}

// RegisterPRPCHandlers registers pPRC handlers.
func RegisterPRPCHandlers(srv *luciserver.Server) error {
	srv.ConfigurePRPC(func(s *prpc.Server) {
		s.AccessControl = prpc.AllowOriginAll
		// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
		s.EnableNonStandardFieldMasks = true
	})
	srv.RegisterUnaryServerInterceptors(span.SpannerDefaultsInterceptor())

	ac, err := analysis.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "creating analysis client").Err()
	}

	cpc, err := changepoints.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "creating changepoint client").Err()
	}

	tvc, err := testverdicts.NewReadClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "creating test verdicts read client").Err()
	}

	trc, err := testresults.NewReadClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "creating test results read client").Err()
	}

	sc, err := sorbet.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "creating sorbet client").Err()
	}

	// May be nil on deployments where Buganizer is not configured.
	bc, err := buganizer.CreateBuganizerClient(srv.Context)
	if err != nil {
		return errors.Annotate(err, "create buganizer client").Err()
	}
	// May be empty on deployments where Buganizer is not configured.
	selfEmail := srv.Context.Value(&buganizer.BuganizerSelfEmailKey).(string)

	adminpb.RegisterAdminServer(srv, admin.NewAdminServer())
	analysispb.RegisterClustersServer(srv, rpc.NewClustersServer(ac))
	analysispb.RegisterMetricsServer(srv, rpc.NewMetricsServer())
	analysispb.RegisterProjectsServer(srv, rpc.NewProjectsServer())
	analysispb.RegisterRulesServer(srv, rpc.NewRulesServer(uiBaseURL(srv), bc, selfEmail))
	analysispb.RegisterTestVariantsServer(srv, rpc.NewTestVariantsServer())
	analysispb.RegisterTestHistoryServer(srv, rpc.NewTestHistoryServer())
	analysispb.RegisterBuganizerTesterServer(srv, rpc.NewBuganizerTesterServer())
	analysispb.RegisterTestVariantBranchesServer(srv, rpc.NewTestVariantBranchesServer(tvc, trc, sc))
	analysispb.RegisterChangepointsServer(srv, rpc.NewChangepointsServer(cpc))
	return nil
}

// RegisterCrons registers cron handlers.
func RegisterCrons(srv *luciserver.Server) {
	updateAnalysisAndBugsHandler := bugscron.NewHandler(srv.Options.CloudProject, uiBaseURL(srv), srv.Options.Prod)
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
	cron.RegisterHandler("schedule-group-changepoints", func(ctx context.Context) error {
		return groupscheduler.CronHandler(ctx, srv.Options.CloudProject)
	})
	cron.RegisterHandler("ensure-views", func(ctx context.Context) error {
		return views.CronHandler(ctx, srv.Options.CloudProject)
	})
	cron.RegisterHandler("merge-test-variant-branches", func(ctx context.Context) error {
		return cpbq.MergeTables(ctx, srv.Options.CloudProject)
	})
	cron.RegisterHandler("update-changepoint-table", func(ctx context.Context) error {
		return bqupdator.UpdateChangepointTable(ctx, srv.Options.CloudProject)
	})
}

// RegisterPubSubHandlers registers pub/sub handlers.
func RegisterPubSubHandlers() {
	pubsub.RegisterJSONPBHandler("buildbucket", app.BuildbucketPubSubHandler)
	pubsub.RegisterJSONPBHandler("cvrun", app.NewCVRunHandler().Handle)
	pubsub.RegisterJSONPBHandler("invocation-finalized", app.NewInvocationFinalizedHandler().Handle)
	pubsub.RegisterJSONPBHandler("invocation-ready-for-export", app.NewInvocationReadyForExportHandler().Handle)
}

// RegisterTaskQueueHandlers registers task queue handlers.
func RegisterTaskQueueHandlers(srv *luciserver.Server) error {
	if err := backfill.RegisterTaskHandler(srv); err != nil {
		return errors.Annotate(err, "register backfill").Err()
	}
	if err := reclustering.RegisterTaskHandler(srv); err != nil {
		return errors.Annotate(err, "register reclustering").Err()
	}
	if err := resultingester.RegisterTaskHandler(srv); err != nil {
		return errors.Annotate(err, "register result ingester").Err()
	}
	if err := verdictingester.RegisterTaskHandler(srv); err != nil {
		return errors.Annotate(err, "register verdict ingester").Err()
	}
	if err := bugupdater.RegisterTaskHandler(srv, uiBaseURL(srv)); err != nil {
		return errors.Annotate(err, "register bug updater").Err()
	}
	if err := changepointgrouper.RegisterTaskHandler(srv); err != nil {
		return errors.Annotate(err, "register changepoint grouper").Err()
	}
	buildjoiner.RegisterTaskHandler()
	return nil
}

func uiBaseURL(srv *luciserver.Server) string {
	return fmt.Sprintf("https://%s.appspot.com", srv.Options.CloudProject)
}
