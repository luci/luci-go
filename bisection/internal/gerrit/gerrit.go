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

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
)

// TODO (aredulla): check if Gerrit actions are enabled in config settings
// for each action

// mockedGerritClientKey is the context key to indicate using mocked
// Gerrit client in tests
var mockedGerritClientKey = "mock Gerrit client"

// Client is the client to communicate with Gerrit
// It wraps a gerritpb.GerritClient
type Client struct {
	gerritClient gerritpb.GerritClient
	host         string
	project      string
}

func newGerritClient(ctx context.Context, host string) (gerritpb.GerritClient, error) {
	if mockClient, ok := ctx.Value(&mockedGerritClientKey).(*gerritpb.MockGerritClient); ok {
		// return a mock Gerrit client for tests
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(gerrit.OAuthScope))
	if err != nil {
		return nil, err
	}

	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// NewClient creates a client to communicate with Gerrit
func NewClient(ctx context.Context, host string, project string) (*Client, error) {
	client, err := newGerritClient(ctx, host)
	if err != nil {
		return nil, errors.Annotate(err, "error making Gerrit client for host %s", host).Err()
	}

	return &Client{
		gerritClient: client,
		host:         host,
		project:      project,
	}, nil
}

// queryChanges gets the info for corresponding change(s) given the query string.
func (c *Client) queryChanges(ctx context.Context, query string) ([]*gerritpb.ChangeInfo, error) {
	req := &gerritpb.ListChangesRequest{
		Query: query,
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_LABELS,
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
			gerritpb.QueryOption_MESSAGES,
			gerritpb.QueryOption_CHANGE_ACTIONS,
			gerritpb.QueryOption_SKIP_MERGEABLE,
			gerritpb.QueryOption_CHECK,
		},
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
func (c *Client) GetChange(ctx context.Context, commitID string) (*gerritpb.ChangeInfo, error) {
	query := fmt.Sprintf("commit:\"%s\"", commitID)
	changes, err := c.queryChanges(ctx, query)
	if err != nil {
		return nil, errors.Annotate(err, "error getting change from Gerrit host %s using query %s",
			c.host, query).Err()
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

// GetReverts gets the corresponding revert(s) for the given change.
func (c *Client) GetReverts(ctx context.Context, change *gerritpb.ChangeInfo) ([]*gerritpb.ChangeInfo, error) {
	query := fmt.Sprintf("project:\"%s\" revertof:%d", change.Project, change.Number)
	changes, err := c.queryChanges(ctx, query)
	if err != nil {
		return nil, errors.Annotate(err, "error getting reverts of a change from Gerrit host %s using query %s",
			c.host, query).Err()
	}

	return changes, nil
}

// CreateRevert creates a revert change in Gerrit for the specified change.
func (c *Client) CreateRevert(ctx context.Context, changeID int64,
	message string) (*gerritpb.ChangeInfo, error) {
	req := &gerritpb.RevertChangeRequest{
		Project: c.project,
		Number:  changeID,
		Message: message,
	}

	// Set timeout to be 2min for creating a revert
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	res, err := c.gerritClient.RevertChange(waitCtx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error creating revert change on Gerrit host %s for change %s~%d",
			c.host, c.project, changeID).Err()
	}

	return res, nil
}

// AddComment adds the given message as a review comment on a change
func (c *Client) AddComment(ctx context.Context, changeID int64, message string) (*gerritpb.ReviewResult, error) {
	req := c.createSetReviewRequest(ctx, changeID, message)
	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error adding comment").Err()
	}

	return res, nil
}

// SendForReview adds the accounts as reviewers for the
// change, and sets the change to be ready for review
func (c *Client) SendForReview(ctx context.Context, changeID int64, message string,
	reviewerAccounts []*gerritpb.AccountInfo, ccAccounts []*gerritpb.AccountInfo) (*gerritpb.ReviewResult, error) {
	req := c.createSetReviewRequest(ctx, changeID, message)

	// Add reviewer and CC accounts to the change
	reviewerCount := len(reviewerAccounts)
	reviewerInputs := make([]*gerritpb.ReviewerInput, reviewerCount+len(ccAccounts))
	for i, account := range reviewerAccounts {
		reviewerInputs[i] = &gerritpb.ReviewerInput{
			Reviewer: account.Email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
		}
	}
	for i, account := range ccAccounts {
		reviewerInputs[reviewerCount+i] = &gerritpb.ReviewerInput{
			Reviewer: account.Email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
		}
	}
	req.Reviewers = reviewerInputs

	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error sending for review").Err()
	}
	return res, nil
}

// Commit bot-commits the change
func (c *Client) Commit(ctx context.Context, changeID int64, message string,
	ccAccounts []*gerritpb.AccountInfo) (*gerritpb.ReviewResult, error) {
	req := c.createSetReviewRequest(ctx, changeID, message)

	// Add CC accounts to the change
	reviewerInputs := make([]*gerritpb.ReviewerInput, len(ccAccounts))
	for i, account := range ccAccounts {
		reviewerInputs[i] = &gerritpb.ReviewerInput{
			Reviewer: account.Email,
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
		}
	}
	req.Reviewers = reviewerInputs

	// Specify the labels required to submit the change to CQ
	req.Labels = map[string]int32{
		"Owners-Override": 1,
		"Bot-Commit":      1,
		"CQ":              2,
	}

	res, err := c.setReview(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error committing").Err()
	}

	return res, nil
}

// createSetReviewRequest is a helper to create a basic SetReviewRequest
func (c *Client) createSetReviewRequest(ctx context.Context, changeID int64, message string) *gerritpb.SetReviewRequest {
	return &gerritpb.SetReviewRequest{
		Number:     changeID,
		Project:    c.project,
		RevisionId: "current",
		Message:    message,
	}
}

// setReview is a helper to call the Gerrit client SetReview function
func (c *Client) setReview(ctx context.Context, req *gerritpb.SetReviewRequest) (*gerritpb.ReviewResult, error) {
	res, err := c.gerritClient.SetReview(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "failed to set review on Gerrit host %s for change %s~%d",
			c.host, req.Project, req.Number).Err()
	}

	return res, nil
}
