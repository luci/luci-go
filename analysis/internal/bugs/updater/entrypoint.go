// Copyright 2023 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

// AnalysisClient is an interface for building and accessing cluster analysis.
type AnalysisClient interface {
	// ReadImpactfulClusters reads analysis for clusters matching the
	// specified criteria.
	ReadImpactfulClusters(ctx context.Context, opts analysis.ImpactfulClusterReadOptions) ([]*analysis.Cluster, error)
}

type UpdateOptions struct {
	UIBaseURL            string
	Project              string
	AnalysisClient       AnalysisClient
	BuganizerClient      buganizer.Client
	SimulateBugUpdates   bool
	MaxBugsFiledPerRun   int
	UpdateRuleBatchSize  int
	ReclusteringProgress *runs.ReclusteringProgress
	RunTimestamp         time.Time
}

// UpdateBugsForProject updates LUCI Analysis-managed bugs for a particular LUCI project.
func UpdateBugsForProject(ctx context.Context, opts UpdateOptions) (retErr error) {
	defer func() {
		// Catch panics, to avoid panics in one project from affecting
		// analysis and bug-filing in another.
		if err := recover(); err != nil {
			logging.Errorf(ctx, "Caught panic updating bugs for project %s: \n %s", opts.Project, string(debug.Stack()))
			retErr = errors.Fmt("caught panic: %v", err)
		}
	}()

	// Bug filing currently don't support chromium milestone projects.
	// Because the bug filing thresholds and priority thresholds are optional in these projects' config.
	if config.ChromiumMilestoneProjectRe.MatchString(opts.Project) {
		return nil
	}
	projectCfg, err := compiledcfg.Project(ctx, opts.Project, opts.ReclusteringProgress.Next.ConfigVersion)
	if err != nil {
		return errors.Fmt("read project config: %w", err)
	}

	mgrs := make(map[string]BugManager)

	if projectCfg.Config.BugManagement.GetBuganizer() != nil {
		if opts.BuganizerClient == nil {
			return errors.New("buganizerClient cannot be nil")
		}

		selfEmail, ok := ctx.Value(&buganizer.BuganizerSelfEmailKey).(string)
		if !ok {
			return errors.New("buganizer self email must be specified")
		}

		// Create Buganizer bug manager
		buganizerBugManager, err := buganizer.NewBugManager(
			opts.BuganizerClient,
			opts.UIBaseURL,
			opts.Project,
			selfEmail,
			projectCfg.Config,
		)
		if err != nil {
			return errors.Fmt("create buganizer bug manager: %w", err)
		}

		mgrs[bugs.BuganizerSystem] = buganizerBugManager
	}

	if len(mgrs) == 0 {
		// No bug managers configured.
		return nil
	}

	bugUpdater := NewBugUpdater(opts.Project, mgrs, opts.AnalysisClient, projectCfg, opts.RunTimestamp)
	bugUpdater.MaxBugsFiledPerRun = opts.MaxBugsFiledPerRun
	if err := bugUpdater.Run(ctx, opts.ReclusteringProgress); err != nil {
		return errors.Fmt("update bugs: %w", err)
	}
	return nil
}
