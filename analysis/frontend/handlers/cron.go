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

package handlers

import (
	"context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bugs/updater"
	"go.chromium.org/luci/analysis/internal/config"
)

// UpdateAnalysisAndBugs handles the update-analysis-and-bugs cron job.
func (h *Handlers) UpdateAnalysisAndBugs(ctx context.Context) error {
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "get config").Err()
	}
	simulate := !h.prod
	enabled := cfg.BugUpdatesEnabled
	err = updater.UpdateAnalysisAndBugs(ctx, cfg.MonorailHostname, h.cloudProject, simulate, enabled)
	if err != nil {
		return errors.Annotate(err, "update bugs").Err()
	}
	return nil
}
