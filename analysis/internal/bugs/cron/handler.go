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

// Package cron defines the update-analysis-and-bugs cron job handler.
// The cron job exists to periodically update cluster analysis
// and to update rules and bugs in response to this analysis.
package cron

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/services/bugupdater"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

// NewHandler initialises a new Handler instance.
func NewHandler(cloudProject, uiBaseURL string, prod bool) *Handler {
	return &Handler{cloudProject: cloudProject, uiBaseURL: uiBaseURL, prod: prod}
}

// Handler handles the update-analysis-and-bugs cron job.
type Handler struct {
	cloudProject string
	uiBaseURL    string
	// prod is set when running in production (not a dev workstation).
	prod bool
}

// CronHandler handles the update-analysis-and-bugs cron job.
func (h *Handler) CronHandler(ctx context.Context) error {
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Fmt("get config: %w", err)
	}
	simulate := !h.prod
	err = updateAnalysisAndBugs(ctx, h.cloudProject, h.uiBaseURL, simulate, cfg.BugUpdatesEnabled)
	if err != nil {
		return errors.Fmt("update bugs: %w", err)
	}
	return nil
}

// updateAnalysisAndBugs updates BigQuery analysis, and then updates bugs
// to reflect this analysis.
// Simulate, if true, avoids any changes being applied to bugs in the
// issue tracker(s) themselves and logs the changes which would be made
// instead. This must be set when running on developer computers as
// LUCI Analysis-initiated monorail changes will appear on monorail
// as the developer themselves rather than the LUCI Analysis service.
// This leads to bugs errounously being detected as having manual priority
// changes.
func updateAnalysisAndBugs(ctx context.Context, gcpProject, uiBaseURL string, simulate, bugUpdatesEnabled bool) (retErr error) {
	runMinute := clock.Now(ctx).Truncate(time.Minute)

	projectCfg, err := config.Projects(ctx)
	if err != nil {
		return err
	}

	analysisClient, err := analysis.NewClient(ctx, gcpProject)
	if err != nil {
		return err
	}
	defer func() {
		if err := analysisClient.Close(); err != nil && retErr == nil {
			retErr = errors.Fmt("closing analysis client: %w", err)
		}
	}()

	if err := analysisClient.RebuildAnalysis(ctx); err != nil {
		return errors.Fmt("update cluster summary analysis: %w", err)
	}

	if bugUpdatesEnabled {
		var errs []error
		for _, project := range projectCfg.Keys() {
			task := &taskspb.UpdateBugs{
				Project:                   project,
				ReclusteringAttemptMinute: timestamppb.New(runMinute),
				// This cron job runs every 15 minutes. Ensure the bug-filing task
				// finishes by the time the next cron job runs.
				Deadline: timestamppb.New(runMinute.Add(15 * time.Minute)),
			}
			if simulate {
				// In local development only, kick off the work to update bugs
				// inline.
				//
				// If you are encountering timeouts in local dev in this step,
				// consider increasing the request timeout by passing to main.go:
				//  -default-request-timeout 15m0s
				h := bugupdater.Handler{
					GCPProject: gcpProject,
					UIBaseURL:  uiBaseURL,
					Simulate:   true,
				}
				if err := h.UpdateBugs(ctx, task); err != nil {
					errs = append(errs, errors.Fmt("update bugs for project %s: %w", project, err))
				}
			} else {
				// In production, create a task queue task to apply the
				// bug updates. This allows us to use the full 15 minutes
				// allotted to updating analysis + bugs intead of being
				// limited by the 10 minute GAE request timeout.
				if err := bugupdater.Schedule(ctx, task); err != nil {
					errs = append(errs, errors.Fmt("schedule bug update task: %w", err))
				}
			}
		}
		if err := errors.Append(errs...); err != nil {
			return err
		}
	}
	// Do last, as this failing should not block bug updates.
	return analysisClient.PurgeStaleRows(ctx)
}
