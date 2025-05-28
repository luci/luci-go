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

package buganizer

import (
	"context"
	"fmt"
	"math/rand"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

func CreateSampleIssue(ctx context.Context) (int64, error) {
	buganizerClientMode := ctx.Value(&BuganizerClientModeKey)
	if buganizerClientMode == nil || buganizerClientMode != ModeProvided {
		return 0, errors.New("Sample issues are only availabe in buganizer Mode \"provided\".")
	}

	componentID := ctx.Value(&BuganizerTestComponentIdKey)

	if componentID == nil {
		return 0, errors.New("Test component id is required to create sample issues.")
	}

	issue := &issuetracker.Issue{
		IssueComment: &issuetracker.IssueComment{
			Comment: fmt.Sprintf("A test issue description %d", rand.Int()),
		},
		IssueState: &issuetracker.IssueState{
			ComponentId: componentID.(int64),
			Type:        issuetracker.Issue_BUG,
			Priority:    issuetracker.Issue_P2,
			Severity:    issuetracker.Issue_S2,
			Title:       fmt.Sprintf("A test issue %d", rand.Int()),
		},
	}
	client, err := NewRPCClient(ctx)

	if err != nil {
		return 0, errors.Fmt("create sample issue rppc client: %w", err)
	}

	createdIssue, err := client.Client.CreateIssue(ctx, &issuetracker.CreateIssueRequest{
		Issue: issue,
	})
	if err != nil {
		return 0, errors.Fmt("create sample issue sending request: %w", err)
	}
	logging.Infof(ctx, "Created a sample issue with id: %d", createdIssue.IssueId)
	return createdIssue.IssueId, nil
}
