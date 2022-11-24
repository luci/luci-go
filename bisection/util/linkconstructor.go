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

	"go.chromium.org/luci/gae/service/info"
)

// ConstructAnalysisURL returns a link to the analysis page in LUCI Bisection
// given a Buildbucket ID
func ConstructAnalysisURL(ctx context.Context, bbid int64) string {
	return fmt.Sprintf("https://%s.appspot.com/analysis/b/%d",
		info.AppID(ctx), bbid)
}

// ConstructBuildURL returns a link to the build page in Milo given a
// Buildbucket ID
func ConstructBuildURL(ctx context.Context, bbid int64) string {
	return fmt.Sprintf("https://ci.chromium.org/b/%d", bbid)
}

// ConstructLUCIBisectionBugURL returns a link to create a bug against
// LUCI Bisection for wrongly blamed commits.
func ConstructLUCIBisectionBugURL(ctx context.Context, analysisURL string,
	commitReviewURL string) string {
	// TODO: update this URL to point to Buganizer

	// Note: the default priority has been set to 3 = Not time critical
	queryParams := url.Values{
		"status":     {"Available"},
		"components": {"Tools>Test>Findit"},
		"labels":     {"LUCI-Bisection-Wrong,Pri-3,Type-Bug"},
		"summary":    {fmt.Sprintf("Wrongly blamed %s", commitReviewURL)},
		"comment":    {fmt.Sprintf("Analysis: %s", analysisURL)},
	}

	return "https://bugs.chromium.org/p/chromium/issues/entry?" + queryParams.Encode()
}
