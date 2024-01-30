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

package util

import (
	"context"
	"fmt"
	"net/url"

	"go.chromium.org/luci/bisection/internal/gerrit"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// ConstructCompileAnalysisURL returns a link to the analysis page in LUCI Bisection
// given a Buildbucket ID
func ConstructCompileAnalysisURL(project string, bbid int64) string {
	return fmt.Sprintf("https://ci.chromium.org/ui/p/%s/bisection/compile-analysis/b/%d", project, bbid)
}

// ConstructTestAnalysisURL returns a link to the analysis page in LUCI Bisection
// given a Buildbucket ID
func ConstructTestAnalysisURL(project string, analysisID int64) string {
	return fmt.Sprintf("https://ci.chromium.org/ui/p/%s/bisection/test-analysis/b/%d", project, analysisID)
}

// ConstructBuildURL returns a link to the build page in Milo given a
// Buildbucket ID
func ConstructBuildURL(ctx context.Context, bbid int64) string {
	return fmt.Sprintf("https://ci.chromium.org/b/%d", bbid)
}

// ConstructBuganizerURLForAnalysis returns a link to create a bug against
// LUCI Bisection for wrongly blamed commits
func ConstructBuganizerURLForAnalysis(commitReviewURL string, analysisURL string) string {
	queryParams := url.Values{
		"title":       {fmt.Sprintf("Wrongly blamed %s", commitReviewURL)},
		"description": {fmt.Sprintf("Analysis: %s", analysisURL)},
		"format":      {"PLAIN"},
		"component":   {"1199205"},
		"type":        {"BUG"},
		"priority":    {"P3"},
	}

	return "http://b.corp.google.com/createIssue?" + queryParams.Encode()
}

func ConstructGerritCodeReviewURL(ctx context.Context,
	gerritClient *gerrit.Client, change *gerritpb.ChangeInfo) string {
	return fmt.Sprintf("https://%s/c/%s/+/%d", gerritClient.Host(ctx),
		change.Project, change.Number)
}

func ConstructTestHistoryURL(project, testID, variantHash string) string {
	queryParams := url.Values{
		"q": {fmt.Sprintf("VHash:%s", variantHash)},
	}
	return fmt.Sprintf("https://ci.chromium.org/ui/test/%s/%s?", url.PathEscape(project), url.PathEscape(testID)) + queryParams.Encode()
}
