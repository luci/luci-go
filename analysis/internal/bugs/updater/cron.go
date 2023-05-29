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
	"runtime/debug"
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
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

var (
	// runsCounter is the metric that counts the number of bug filing
	// runs by LUCI Analysis, by project and outcome.
	runsCounter = metric.NewCounter("analysis/bug_updater/runs",
		"The number of auto-bug filing runs completed, "+
			"by LUCI Project and status.",
		&types.MetricMetadata{
			Units: "runs",
		},
		// The LUCI project.
		field.String("project"),
		// The run status.
		field.String("status"), // success | failure
	)

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
	RebuildAnalysis(ctx context.Context) error
	// ReadImpactfulClusters reads analysis for clusters matching the
	// specified criteria.
	ReadImpactfulClusters(ctx context.Context, opts analysis.ImpactfulClusterReadOptions) ([]*analysis.Cluster, error)
	// PurgeStaleRows purges stale clustered failure rows
	// from the table.
	PurgeStaleRows(ctx context.Context) error
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
	for _, project := range projectCfg.Keys() {
		// Until each project succeeds, report "failure".
		statusByProject.Store(project, "failure")
	}
	defer func() {
		statusByProject.Range(func(key, value any) bool {
			project := key.(string)
			status := value.(string)
			statusGauge.Set(ctx, status, project)
			runsCounter.Add(ctx, 1, project, status)
			return true // continue iteration
		})
	}()

	monorailClient, err := monorail.NewClient(ctx, monorailHost)
	if err != nil {
		return err
	}

	analysisClient, err := analysis.NewClient(ctx, gcpProject)
	if err != nil {
		return err
	}
	defer func() {
		if err := analysisClient.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing analysis client").Err()
		}
	}()

	buganizerClient, err := createBuganizerClient(ctx)

	if err != nil {
		return errors.Annotate(err, "creating a buganizer client").Err()
	}

	if buganizerClient != nil {
		defer buganizerClient.Close()
	}

	// Capture the current state of re-clustering before running analysis.
	// This will reflect how up-to-date our analysis is when it completes.
	projectProgress := make(map[string]*runs.ReclusteringProgress)
	for _, project := range projectCfg.Keys() {
		progress, err := runs.ReadReclusteringProgress(ctx, project)
		if err != nil {
			return errors.Annotate(err, "read re-clustering progress").Err()
		}
		projectProgress[project] = progress
	}

	if err := analysisClient.RebuildAnalysis(ctx); err != nil {
		return errors.Annotate(err, "update cluster summary analysis").Err()
	}

	taskGenerator := func(c chan<- func() error) {
		for _, project := range projectCfg.Keys() {
			opts := updateOptions{
				appID:                gcpProject,
				project:              project,
				analysisClient:       analysisClient,
				monorailClient:       monorailClient,
				buganizerClient:      buganizerClient,
				simulateBugUpdates:   simulate,
				maxBugsFiledPerRun:   1,
				reclusteringProgress: projectProgress[project],
			}

			// Assign project to local variable to ensure it can be
			// accessed correctly inside function closures.
			project := project
			c <- func() error {
				// Isolate other projects from bug update errors
				// in one project.
				start := time.Now()
				err := updateBugsForProject(ctx, opts)
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
	if enable {
		if err := parallel.WorkPool(workerCount, taskGenerator); err != nil {
			return err
		}
	}
	// Do last, as this failing should not block bug updates.
	return analysisClient.PurgeStaleRows(ctx)
}

func createBuganizerClient(ctx context.Context) (buganizer.Client, error) {
	buganizerClientMode := ctx.Value(&buganizer.BuganizerClientModeKey)
	var buganizerClient buganizer.Client
	var err error
	if buganizerClientMode != nil {
		switch buganizerClientMode {
		case buganizer.ModeProvided:
			// TODO (b/263906102)
			buganizerClient, err = buganizer.NewRPCClient(ctx)
			if err != nil {
				return nil, errors.Annotate(err, "create new buganizer client").Err()
			}
		case buganizer.ModeDisable:
			break
		default:
			return nil, errors.New("Unrecognized buganizer-mode value used.")
		}
	}

	return buganizerClient, err
}

type updateOptions struct {
	appID                string
	project              string
	analysisClient       AnalysisClient
	monorailClient       *monorail.Client
	buganizerClient      buganizer.Client
	simulateBugUpdates   bool
	maxBugsFiledPerRun   int
	reclusteringProgress *runs.ReclusteringProgress
}

// updateBugsForProject updates LUCI Analysis-managed bugs for a particular LUCI project.
func updateBugsForProject(ctx context.Context, opts updateOptions) (retErr error) {
	defer func() {
		// Catch panics, to avoid panics in one project from affecting
		// analysis and bug-filing in another.
		if err := recover(); err != nil {
			logging.Errorf(ctx, "Caught panic updating bugs for project %s: \n %s", opts.project, string(debug.Stack()))
			retErr = errors.Reason("caught panic: %v", err).Err()
		}
	}()

	// Bug filing currently don't support chromium milestone projects.
	// Because the bug filing thresholds and priority thresholds are optional in these projects' config.
	if config.ChromiumMilestoneProjectRe.MatchString(opts.project) {
		return nil
	}
	projectCfg, err := compiledcfg.Project(ctx, opts.project, opts.reclusteringProgress.Next.ConfigVersion)
	if err != nil {
		return errors.Annotate(err, "read project config").Err()
	}
	hasBugSystem := projectCfg.Config.Monorail != nil || projectCfg.Config.Buganizer != nil
	if !hasBugSystem {
		return nil
	}
	mgrs := make(map[string]BugManager)

	if projectCfg.Config.Monorail != nil {
		// Create Monorail bug manager
		monorailBugManager, err := monorail.NewBugManager(opts.monorailClient, opts.appID, opts.project, projectCfg.Config)
		if err != nil {
			return errors.Annotate(err, "create monorail bug manager").Err()
		}

		monorailBugManager.Simulate = opts.simulateBugUpdates
		mgrs[bugs.MonorailSystem] = monorailBugManager

	}
	if projectCfg.Config.Buganizer != nil {
		if opts.buganizerClient == nil {
			return errors.New("buganizerClient cannot be nil.")
		}
		// Create Buganizer bug manager
		buganizerBugManager := buganizer.NewBugManager(
			opts.buganizerClient,
			opts.appID,
			opts.project,
			projectCfg.Config,
			opts.simulateBugUpdates,
		)

		mgrs[bugs.BuganizerSystem] = buganizerBugManager
	}
	bugUpdater := NewBugUpdater(opts.project, mgrs, opts.analysisClient, projectCfg)
	bugUpdater.MaxBugsFiledPerRun = opts.maxBugsFiledPerRun
	if err := bugUpdater.Run(ctx, opts.reclusteringProgress); err != nil {
		return errors.Annotate(err, "update bugs").Err()
	}
	return nil
}
