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

package monorail

import (
	"context"
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
)

var testMonorailClientKey = "used in tests only for setting the monorail client test double"

// maxCommentPageSize is the maximum number of comments that can be returned
// by Monorail in one go.
const maxCommentPageSize = 100

func newClient(ctx context.Context, host string) (*prpc.Client, error) {
	// Reference: go/dogfood-monorail-v3-api
	apiHost := fmt.Sprintf("api-dot-%v", host)
	audience := fmt.Sprintf("https://%v", host)

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithIDTokenAudience(audience))
	if err != nil {
		return nil, err
	}
	// httpClient is able to make HTTP requests authenticated with
	// ID tokens.
	httpClient := &http.Client{Transport: t}
	monorailPRPCClient := &prpc.Client{
		C:    httpClient,
		Host: apiHost,
	}
	return monorailPRPCClient, nil
}

// Creates a new Monorail client. Host is the monorail host to use,
// e.g. monorail-prod.appspot.com.
func NewClient(ctx context.Context, host string) (*Client, error) {
	if testClient, ok := ctx.Value(&testMonorailClientKey).(*Client); ok {
		return testClient, nil
	}

	client, err := newClient(ctx, host)
	if err != nil {
		return nil, err
	}

	updateClient, err := newClient(ctx, host)
	if err != nil {
		return nil, err
	}
	// We do not want to retry monorail operations that mutate issues.
	// When monorail has an outage, the updates may occur despite
	// the service returning an exception. This may result in
	// bug creation and update spam, which we would like to avoid.
	updateClient.Options = &prpc.Options{Retry: retry.None}

	return &Client{
		updateIssuesClient: mpb.NewIssuesPRPCClient(updateClient),
		issuesClient:       mpb.NewIssuesPRPCClient(client),
		projectsClient:     mpb.NewProjectsPRPCClient(client),
	}, nil
}

// Client is a client to communicate with the Monorail issue tracker.
type Client struct {
	updateIssuesClient mpb.IssuesClient
	issuesClient       mpb.IssuesClient
	projectsClient     mpb.ProjectsClient
}

// GetIssue retrieves the details of a monorail issue. Name should
// follow the format "projects/<projectid>/issues/<issueid>".
func (c *Client) GetIssue(ctx context.Context, name string) (*mpb.Issue, error) {
	req := mpb.GetIssueRequest{Name: name}
	resp, err := c.issuesClient.GetIssue(ctx, &req)
	if err != nil {
		return nil, errors.Annotate(err, "GetIssue %q", name).Err()
	}
	return resp, nil
}

// BatchGetIssues gets the details of the specified monorail issues.
// At most 100 issues can be queried at once. It is guaranteed
// that the i_th issue in the result will match the i_th issue
// requested. It is valid to request the same issue multiple
// times in the same request.
func (c *Client) BatchGetIssues(ctx context.Context, names []string) ([]*mpb.Issue, error) {
	var deduplicatedNames []string
	requestedNames := make(map[string]bool)
	for _, name := range names {
		if !requestedNames[name] {
			deduplicatedNames = append(deduplicatedNames, name)
			requestedNames[name] = true
		}
	}
	req := mpb.BatchGetIssuesRequest{Names: deduplicatedNames}
	resp, err := c.issuesClient.BatchGetIssues(ctx, &req)
	if err != nil {
		return nil, errors.Annotate(err, "BatchGetIssues %v", deduplicatedNames).Err()
	}
	issuesByName := make(map[string]*mpb.Issue)
	for _, issue := range resp.Issues {
		issuesByName[issue.Name] = issue
	}
	var result []*mpb.Issue
	for _, name := range names {
		// Copy the proto to avoid an issue being aliased in
		// the result if the same issue is requested multiple times.
		// The caller should be able to assume each issue returned
		// is a distinct object.
		issue := &mpb.Issue{}
		proto.Merge(issue, issuesByName[name])
		result = append(result, issue)
	}
	return result, nil
}

// MakeIssue creates the given issue in monorail, adding the specified
// description.
func (c *Client) MakeIssue(ctx context.Context, req *mpb.MakeIssueRequest) (*mpb.Issue, error) {
	issue, err := c.updateIssuesClient.MakeIssue(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "MakeIssue").Err()
	}
	return issue, err
}

// ListComments lists comments present on the given issue. At most
// 1000 comments are returned.
func (c *Client) ListComments(ctx context.Context, name string) ([]*mpb.Comment, error) {
	var result []*mpb.Comment

	pageToken := ""

	// Scan at most 10 pages.
	for p := 0; p < 10; p++ {
		req := mpb.ListCommentsRequest{
			Parent:    name,
			PageSize:  maxCommentPageSize,
			PageToken: pageToken,
		}
		resp, err := c.issuesClient.ListComments(ctx, &req)
		if err != nil {
			return nil, errors.Annotate(err, "ListComments %q", name).Err()
		}
		result = append(result, resp.Comments...)
		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return result, nil
}

// ModifyIssues modifies the given issue.
func (c *Client) ModifyIssues(ctx context.Context, req *mpb.ModifyIssuesRequest) error {
	_, err := c.updateIssuesClient.ModifyIssues(ctx, req)
	if err != nil {
		return errors.Annotate(err, "ModifyIssues").Err()
	}
	return nil
}

// GetComponentExistsAndActive returns true if the given component exists
// and is active in monorail.
func (c *Client) GetComponentExistsAndActive(ctx context.Context, project string, component string) (bool, error) {
	request := &mpb.GetComponentDefRequest{
		Name: fmt.Sprintf("projects/%s/componentDefs/%s", project, component),
	}
	response, err := c.projectsClient.GetComponentDef(ctx, request)
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, errors.Annotate(err, "fetching components").Err()
	}
	return response.State == mpb.ComponentDef_ACTIVE, nil
}
