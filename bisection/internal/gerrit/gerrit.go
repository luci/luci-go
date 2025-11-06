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

// Package gerrit contains logic for interacting with Gerrit
package gerrit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
)

// The options to use when querying Gerrit for changes
var queryOptions = []gerritpb.QueryOption{
	gerritpb.QueryOption_LABELS,
	gerritpb.QueryOption_CURRENT_REVISION,
	gerritpb.QueryOption_CURRENT_COMMIT,
	gerritpb.QueryOption_DETAILED_ACCOUNTS,
	gerritpb.QueryOption_MESSAGES,
	gerritpb.QueryOption_CHANGE_ACTIONS,
	gerritpb.QueryOption_SKIP_MERGEABLE,
	gerritpb.QueryOption_CHECK,
}

// mockedGerritClientKey is the context key to indicate using mocked
// Gerrit client in tests
var mockedGerritClientKey = "mock Gerrit client"

// Client is the client to communicate with Gerrit
// It wraps a gerritpb.GerritClient
type Client struct {
	gerritClient gerritpb.GerritClient
	host         string
}

func newGerritClient(ctx context.Context, host string) (gerritpb.GerritClient, error) {
	if mockClient, ok := ctx.Value(&mockedGerritClientKey).(*gerritpb.MockGerritClient); ok {
		// return a mock Gerrit client for tests
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.GerritScopeSet()...))
	if err != nil {
		return nil, err
	}

	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// NewClient creates a client to communicate with Gerrit
func NewClient(ctx context.Context, host string) (*Client, error) {
	client, err := newGerritClient(ctx, host)
	if err != nil {
		return nil, errors.Fmt("error making Gerrit client for host %s: %w", host, err)
	}

	return &Client{
		gerritClient: client,
		host:         host,
	}, nil
}

// Host returns the Gerrit host string
func (c *Client) Host(ctx context.Context) string {
	return c.host
}

// queryChanges gets the info for corresponding change(s) given the query string.
func (c *Client) queryChanges(ctx context.Context, query string) ([]*gerritpb.ChangeInfo, error) {
	req := &gerritpb.ListChangesRequest{
		Query:   query,
		Options: queryOptions,
	}

	res, err := c.gerritClient.ListChanges(ctx, req)
	if err != nil {
		return nil, err
	}

	return res.Changes, nil
}

// GetChange gets the corresponding change info given the commit ID.
// This function returns an error if none or more than 1 changes are returned
// by Gerrit.
func (c *Client) GetChange(ctx context.Context, project string, commitID string) (*gerritpb.ChangeInfo, error) {
	query := fmt.Sprintf("project:\"%s\" commit:\"%s\"", project, commitID)
	changes, err := c.queryChanges(ctx, query)
	if err != nil {
		return nil, errors.Fmt("error getting change from Gerrit host %s using query %s: %w",
			c.host, query, err)
	}

	if len(changes) == 0 {
		return nil, fmt.Errorf("no change found from Gerrit host %s using query %s",
			c.host, query,
		)
	}

	if len(changes) > 1 {
		return nil, fmt.Errorf("multiple changes found from Gerrit host %s using query %s",
			c.host, query,
		)
	}

	return changes[0], nil
}

// RefetchChange queries Gerrit for the given change, and returns the latest
// state of the change
func (c *Client) RefetchChange(ctx context.Context, change *gerritpb.ChangeInfo) (*gerritpb.ChangeInfo, error) {
	req := &gerritpb.GetChangeRequest{
		Project: change.Project,
		Number:  change.Number,
		Options: queryOptions,
	}

	res, err := c.gerritClient.GetChange(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// GetReverts gets the corresponding revert(s) for the given change.
func (c *Client) GetReverts(ctx context.Context, change *gerritpb.ChangeInfo) ([]*gerritpb.ChangeInfo, error) {
	query := fmt.Sprintf("project:\"%s\" revertof:%d", change.Project, change.Number)
	changes, err := c.queryChanges(ctx, query)
	if err != nil {
		return nil, errors.Fmt("error getting reverts of a change from Gerrit host %s using query %s: %w",
			c.host, query, err)
	}

	return changes, nil
}

// HasDependency returns whether the change has another merged change depending on it
func (c *Client) HasDependency(ctx context.Context, change *gerritpb.ChangeInfo) (bool, error) {
	relatedChanges, err := c.getRelatedChanges(ctx, change)
	if err != nil {
		return false, errors.Fmt("failed checking dependency: %w", err)
	}

	for _, relatedChange := range relatedChanges {
		if relatedChange.Status == gerritpb.ChangeStatus_MERGED {
			// relatedChange here is the newest merged. If relatedChange != change,
			// then there is a merged dependency
			return relatedChange.Project != change.Project ||
				relatedChange.Number != change.Number, nil
		}
	}

	// none of the related changes are merged, so no merged dependencies
	return false, nil
}

// CreateRevert creates a revert change in Gerrit for the specified change.
func (c *Client) CreateRevert(ctx context.Context, change *gerritpb.ChangeInfo, message string) (*gerritpb.ChangeInfo, error) {
	logging.Debugf(ctx, "gerrit Client.CreateRevert message: '%s'", message)
	req := &gerritpb.RevertChangeRequest{
		Project: change.Project,
		Number:  change.Number,
		Message: message,
	}

	// Set timeout for creating a revert
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*1)
	defer cancel()

	res, err := c.gerritClient.RevertChange(waitCtx, req)
	if err != nil {
		return nil, errors.Fmt("error creating revert change on Gerrit host %s for change %s~%d: %w",
			c.host, req.Project, req.Number, err)
	}

	return res, nil
}

// AddComment adds the given message as a review comment on a change
func (c *Client) AddComment(ctx context.Context, change *gerritpb.ChangeInfo, message string) (*gerritpb.ReviewResult, error) {
	req := c.createSetReviewRequest(ctx, change, message)
	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Fmt("error adding comment: %w", err)
	}

	return res, nil
}

// SendForReview adds the emails as reviewers for the
// change, and sets the change to be ready for review
func (c *Client) SendForReview(ctx context.Context, change *gerritpb.ChangeInfo, message string,
	reviewerEmails []string, ccEmails []string) (*gerritpb.ReviewResult, error) {
	req := c.createSetReviewRequest(ctx, change, message)

	// Add reviewer and CC emails to the change
	reviewerCount := len(reviewerEmails)
	reviewerInputs := make([]*gerritpb.ReviewerInput, reviewerCount+len(ccEmails))
	for i, email := range reviewerEmails {
		reviewerInputs[i] = &gerritpb.ReviewerInput{
			Reviewer: email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
		}
	}
	for i, email := range ccEmails {
		reviewerInputs[reviewerCount+i] = &gerritpb.ReviewerInput{
			Reviewer: email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
		}
	}
	req.Reviewers = reviewerInputs

	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Fmt("error sending for review: %w", err)
	}
	return res, nil
}

// CommitRevert bot-commits the revert change. The change must be a pure revert;
// if not, this function does not attempt to commit the change and returns an error.
func (c *Client) CommitRevert(ctx context.Context, change *gerritpb.ChangeInfo,
	message string, ccEmails []string) (*gerritpb.ReviewResult, error) {
	// Check the change is a pure revert
	isRevert, err := c.isPureRevert(ctx, change)
	if err != nil {
		return nil, err
	}

	if !isRevert {
		return nil, fmt.Errorf(
			"failed to commit change on Gerrit host %s - change %s~%d is not a pure revert",
			c.host, change.Project, change.Number)
	}

	req := c.createSetReviewRequest(ctx, change, message)

	// Add CC emails to the change
	reviewerInputs := make([]*gerritpb.ReviewerInput, len(ccEmails))
	for i, email := range ccEmails {
		reviewerInputs[i] = &gerritpb.ReviewerInput{
			Reviewer: email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
		}
	}
	req.Reviewers = reviewerInputs

	// Specify the labels required to submit the change to CQ
	req.Labels = map[string]int32{
		"Owners-Override": 1,
		"Bot-Commit":      1,
		"Commit-Queue":    2,
	}

	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Fmt("error committing: %w", err)
	}

	return res, nil
}

// createSetReviewRequest is a helper to create a basic SetReviewRequest
func (c *Client) createSetReviewRequest(ctx context.Context, change *gerritpb.ChangeInfo,
	message string) *gerritpb.SetReviewRequest {
	return &gerritpb.SetReviewRequest{
		Project:    change.Project,
		Number:     change.Number,
		RevisionId: "current",
		Message:    message,
	}
}

// getRelatedChanges is a helper to call the Gerrit client GetRelatedChanges function
func (c *Client) getRelatedChanges(ctx context.Context, change *gerritpb.ChangeInfo) ([]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, error) {
	req := &gerritpb.GetRelatedChangesRequest{
		Project:    change.Project,
		Number:     change.Number,
		RevisionId: "current",
	}

	res, err := c.gerritClient.GetRelatedChanges(ctx, req)
	if err != nil {
		return nil, errors.Fmt("failed getting related changes from Gerrit host %s for change %s~%d: %w",
			c.host, req.Project, req.Number, err)
	}

	// Changes are sorted by git commit order, newest to oldest.
	// Empty if there are no related changes. See:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#related-changes-info
	return res.Changes, nil
}

// isPureRevert is a helper to call the Gerrit client GetPureRevert function,
// and returns whether the change is a pure revert of the change
// referenced in its "revertOf" field. See:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-pure-revert
func (c *Client) isPureRevert(ctx context.Context, change *gerritpb.ChangeInfo) (bool, error) {
	req := &gerritpb.GetPureRevertRequest{
		Project: change.Project,
		Number:  change.Number,
	}

	res, err := c.gerritClient.GetPureRevert(ctx, req)
	if err != nil {
		return false, errors.Fmt("error querying Gerrit host %s on whether the change %s~%d is a pure revert: %w",
			c.host, req.Project, req.Number, err)
	}

	return res.IsPureRevert, nil
}

// setReview is a helper to call the Gerrit client SetReview function
func (c *Client) setReview(ctx context.Context, req *gerritpb.SetReviewRequest) (*gerritpb.ReviewResult, error) {
	res, err := c.gerritClient.SetReview(ctx, req)
	if err != nil {
		return nil, errors.Fmt("failed to set review on Gerrit host %s for change %s~%d: %w",
			c.host, req.Project, req.Number, err)
	}

	return res, nil
}
