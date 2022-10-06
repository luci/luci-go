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

package updater

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

var (
	// statusGauge reports the most recent status of the bug updater job.
	// Reports either "success" or "failure".
	statusGauge = metric.NewString("analysis/bug_updater/status",
		"Whether automatic bug updates are succeeding, by LUCI Project.",
		nil,
		// The LUCI project.
		field.String("project"),
	)

	durationGauge = metric.NewFloat("analysis/bug_updater/duration",
		"How long it is taking to update bugs, by LUCI Project.",
		&types.MetricMetadata{
			Units: types.Seconds,
		},
		// The LUCI project.
		field.String("project"),
	)
)

// workerCount is the number of workers to use to update
// analysis and bugs for different LUCI Projects concurrently.
const workerCount = 8

// AnalysisClient is an interface for building and accessing cluster analysis.
type AnalysisClient interface {
	// RebuildAnalysis rebuilds analysis from the latest clustered test
	// results.
	RebuildAnalysis(ctx context.Context, project string) error
	// ReadImpactfulClusters reads analysis for clusters matching the
	// specified criteria.
	ReadImpactfulClusters(ctx context.Context, opts analysis.ImpactfulClusterReadOptions) ([]*analysis.Cluster, error)
	// PurgeStaleRows purges stale clustered failure rows
	// from the table.
	PurgeStaleRows(ctx context.Context, luciProject string) error
}

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, statusGauge, durationGauge)
}

// NewHandler initialises a new Handler instance.
func NewHandler(cloudProject string, prod bool) *Handler {
	return &Handler{cloudProject: cloudProject, prod: prod}
}

// Handler handles the update-analysis-and-bugs cron job.
type Handler struct {
	cloudProject string
	// prod is set when running in production (not a dev workstation).
	prod bool
}

// CronHandler handles the update-analysis-and-bugs cron job.
func (h *Handler) CronHandler(ctx context.Context) error {
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "get config").Err()
	}
	simulate := !h.prod
	enabled := cfg.BugUpdatesEnabled
	err = updateAnalysisAndBugs(ctx, cfg.MonorailHostname, h.cloudProject, simulate, enabled)
	if err != nil {
		return errors.Annotate(err, "update bugs").Err()
	}
	return nil
}

// updateAnalysisAndBugs updates BigQuery analysis, and then updates bugs
// to reflect this analysis.
// Simulate, if true, avoids any changes being applied to monorail and logs
// the changes which would be made instead. This must be set when running
// on developer computers as LUCI Analysis-initiated monorail changes will
// appear on monorail as the developer themselves rather than the
// LUCI Analysis service.
// This leads to bugs errounously being detected as having manual priority
// changes.
func updateAnalysisAndBugs(ctx context.Context, monorailHost, gcpProject string, simulate, enable bool) (retErr error) {
	projectCfg, err := config.Projects(ctx)
	if err != nil {
		return err
	}

	statusByProject := &sync.Map{}
	for project := range projectCfg {
		// Until each project succeeds, report "failure".
		statusByProject.Store(project, "failure")
	}
	defer func() {
		statusByProject.Range(func(key, value interface{}) bool {
			project := key.(string)
			status := value.(string)
			statusGauge.Set(ctx, status, project)
			return true // continue iteration
		})
	}()

	mc, err := monorail.NewClient(ctx, monorailHost)
	if err != nil {
		return err
	}

	ac, err := analysis.NewClient(ctx, gcpProject)
	if err != nil {
		return err
	}
	defer func() {
		if err := ac.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing analysis client").Err()
		}
	}()

	projectsWithDataset, err := ac.ProjectsWithDataset(ctx)
	if err != nil {
		return errors.Annotate(err, "querying projects with dataset").Err()
	}

	taskGenerator := func(c chan<- func() error) {
		for project := range projectCfg {
			if _, ok := projectsWithDataset[project]; !ok {
				// Dataset not provisioned for project.
				statusByProject.Store(project, "disabled")
				continue
			}

			opts := updateOptions{
				appID:              gcpProject,
				project:            project,
				analysisClient:     ac,
				monorailClient:     mc,
				simulateBugUpdates: simulate,
				enableBugUpdates:   enable,
				maxBugsFiledPerRun: 1,
			}
			// Assign project to local variable to ensure it can be
			// accessed correctly inside function closures.
			project := project
			c <- func() error {
				// Isolate other projects from bug update errors
				// in one project.
				start := time.Now()
				err := updateAnalysisAndBugsForProject(ctx, opts)
				if err != nil {
					err = errors.Annotate(err, "in project %v", project).Err()
					logging.Errorf(ctx, "Updating analysis and bugs: %s", err)
				} else {
					statusByProject.Store(project, "success")
				}
				elapsed := time.Since(start)
				durationGauge.Set(ctx, elapsed.Seconds(), project)

				// Let the cron job succeed even if one of the projects
				// is failing. Cron job should only fail if something
				// catastrophic happens (e.g. such that metrics may
				// fail to be reported).
				return nil
			}
		}
	}

	return parallel.WorkPool(workerCount, taskGenerator)
}

type updateOptions struct {
	appID              string
	project            string
	analysisClient     AnalysisClient
	monorailClient     *monorail.Client
	enableBugUpdates   bool
	simulateBugUpdates bool
	maxBugsFiledPerRun int
}

// updateAnalysisAndBugsForProject updates BigQuery analysis, and
// LUCI Analysis-managed bugs for a particular LUCI project.
func updateAnalysisAndBugsForProject(ctx context.Context, opts updateOptions) error {
	// Capture the current state of re-clustering before running analysis.
	// This will reflect how up-to-date our analysis is when it completes.
	progress, err := runs.ReadReclusteringProgress(ctx, opts.project)
	if err != nil {
		return errors.Annotate(err, "read re-clustering progress").Err()
	}

	projectCfg, err := compiledcfg.Project(ctx, opts.project, progress.Next.ConfigVersion)
	if err != nil {
		return errors.Annotate(err, "read project config").Err()
	}

	if err := opts.analysisClient.RebuildAnalysis(ctx, opts.project); err != nil {
		return errors.Annotate(err, "update cluster summary analysis").Err()
	}
	if opts.enableBugUpdates {
		mgrs := make(map[string]BugManager)

		mbm, err := monorail.NewBugManager(opts.monorailClient, opts.appID, opts.project, projectCfg.Config)
		if err != nil {
			return errors.Annotate(err, "create monorail bug manager").Err()
		}

		mbm.Simulate = opts.simulateBugUpdates
		mgrs[bugs.MonorailSystem] = mbm

		bu := NewBugUpdater(opts.project, mgrs, opts.analysisClient, projectCfg)
		bu.MaxBugsFiledPerRun = opts.maxBugsFiledPerRun
		if err := bu.Run(ctx, progress); err != nil {
			return errors.Annotate(err, "update bugs").Err()
		}
	}
	// Do last, as this failing should not block bug updates.
	if err := opts.analysisClient.PurgeStaleRows(ctx, opts.project); err != nil {
		return errors.Annotate(err, "purge stale rows").Err()
	}
	return nil
}
