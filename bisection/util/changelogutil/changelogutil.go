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

// Package changelogutil contains utility functions for changelogs.
package changelogutil

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	lbpb "go.chromium.org/luci/bisection/proto"
)

// GetChangeLogs queries Gitiles for changelogs in the regression range
func GetChangeLogs(c context.Context, rr *lbpb.RegressionRange) ([]*model.ChangeLog, error) {
	if rr.LastPassed.Host != rr.FirstFailed.Host || rr.LastPassed.Project != rr.FirstFailed.Project {
		return nil, fmt.Errorf("RepoURL for last pass and first failed commits must be same, but aren't: %v and %v", rr.LastPassed, rr.FirstFailed)
	}
	repoURL := gitiles.GetRepoUrl(c, rr.FirstFailed)
	return gitiles.GetChangeLogs(c, repoURL, rr.LastPassed.Id, rr.FirstFailed.Id)
}
