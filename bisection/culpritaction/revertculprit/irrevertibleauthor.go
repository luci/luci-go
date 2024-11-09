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

package revertculprit

import (
	"context"
	"regexp"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/bisection/internal/gerrit"
)

// HasIrrevertibleAuthor returns whether the change's commit author is one that
// should not have any of their changes reverted, e.g. DEPS rollers that auto-commit code.
func HasIrrevertibleAuthor(ctx context.Context, change *gerritpb.ChangeInfo) (bool, error) {
	author, err := gerrit.AuthorEmail(ctx, change)
	if err != nil {
		return false, err
	}

	// TODO (nqmtuan): move the explicit irrevertible emails below to configs
	switch author {
	case
		"blink-w3c-test-autoroller@chromium.org",
		"chrome-release-bot@chromium.org",
		"chromeos-commit-bot@chromium.org",
		"ios-autoroll@chromium.org",
		"v8-autoroll@chromium.org",
		"v8-ci-autoroll-builder@chops-service-accounts.iam.gserviceaccount.com":
		return true, nil
	default:
		// not an exact match of above
	}

	// check the email pattern instead
	pattern := regexp.MustCompile(
		`.*chromium.*-autoroll@skia-(corp|public|buildbots)(\.google\.com)?\.iam\.gserviceaccount\.com`,
	)
	return pattern.MatchString(author), nil
}
