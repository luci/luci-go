// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package issuetracker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	httptransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	issuetrackerpb "go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

var newClientHook clientHook

// CallOptions contains the retry settings for each method of Client.
type CallOptions struct {
	GetComponent            []gax.CallOption
	ListIssues              []gax.CallOption
	BatchGetIssues          []gax.CallOption
	GetIssue                []gax.CallOption
	CreateIssue             []gax.CallOption
	ModifyIssue             []gax.CallOption
	CreateIssueRelationship []gax.CallOption
	ListIssueRelationships  []gax.CallOption
	ListIssueUpdates        []gax.CallOption
	CreateIssueComment      []gax.CallOption
	ListIssueComments       []gax.CallOption
	UpdateIssueComment      []gax.CallOption
	ListAttachments         []gax.CallOption
	CreateHotlistEntry      []gax.CallOption
	DeleteHotlistEntry      []gax.CallOption
	GetAutomationAccess     []gax.CallOption
}

func defaultGRPCClientOptions() []option.ClientOption {
	return []option.ClientOption{
		internaloption.WithDefaultEndpoint("placeholder-issuetracker-c2p.googleapis.com:443"),
		internaloption.WithDefaultMTLSEndpoint("placeholder-issuetracker-c2p.mtls.googleapis.com:443"),
		internaloption.WithDefaultAudience("https://placeholder-issuetracker-c2p.googleapis.com/"),
		internaloption.WithDefaultScopes(DefaultAuthScopes()...),
		internaloption.EnableJwtWithScope(),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(math.MaxInt32))),
	}
}

func defaultCallOptions() *CallOptions {
	return &CallOptions{
		GetComponent: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		ListIssues: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		BatchGetIssues: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		GetIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		CreateIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		ModifyIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		CreateIssueRelationship: []gax.CallOption{},
		ListIssueRelationships:  []gax.CallOption{},
		ListIssueUpdates:        []gax.CallOption{},
		CreateIssueComment: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		ListIssueComments: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		UpdateIssueComment: []gax.CallOption{},
		ListAttachments: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable,
				}, gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				})
			}),
		},
		CreateHotlistEntry:  []gax.CallOption{},
		DeleteHotlistEntry:  []gax.CallOption{},
		GetAutomationAccess: []gax.CallOption{},
	}
}

func defaultRESTCallOptions() *CallOptions {
	return &CallOptions{
		GetComponent: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		ListIssues: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		BatchGetIssues: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		GetIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		CreateIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		ModifyIssue: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		CreateIssueRelationship: []gax.CallOption{},
		ListIssueRelationships:  []gax.CallOption{},
		ListIssueUpdates:        []gax.CallOption{},
		CreateIssueComment: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
		},
		ListIssueComments: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		UpdateIssueComment: []gax.CallOption{},
		ListAttachments: []gax.CallOption{
			gax.WithTimeout(60000 * time.Millisecond),
			gax.WithRetry(func() gax.Retryer {
				return gax.OnHTTPCodes(gax.Backoff{
					Initial:    1000 * time.Millisecond,
					Max:        10000 * time.Millisecond,
					Multiplier: 1.30,
				},
					http.StatusServiceUnavailable)
			}),
		},
		CreateHotlistEntry:  []gax.CallOption{},
		DeleteHotlistEntry:  []gax.CallOption{},
		GetAutomationAccess: []gax.CallOption{},
	}
}

// internalClient is an interface that defines the methods available from Private Issue Tracker API.
type internalClient interface {
	Close() error
	setGoogleClientInfo(...string)
	Connection() *grpc.ClientConn
	GetComponent(context.Context, *issuetrackerpb.GetComponentRequest, ...gax.CallOption) (*issuetrackerpb.Component, error)
	ListIssues(context.Context, *issuetrackerpb.ListIssuesRequest, ...gax.CallOption) *IssueIterator
	BatchGetIssues(context.Context, *issuetrackerpb.BatchGetIssuesRequest, ...gax.CallOption) (*issuetrackerpb.BatchGetIssuesResponse, error)
	GetIssue(context.Context, *issuetrackerpb.GetIssueRequest, ...gax.CallOption) (*issuetrackerpb.Issue, error)
	CreateIssue(context.Context, *issuetrackerpb.CreateIssueRequest, ...gax.CallOption) (*issuetrackerpb.Issue, error)
	ModifyIssue(context.Context, *issuetrackerpb.ModifyIssueRequest, ...gax.CallOption) (*issuetrackerpb.Issue, error)
	CreateIssueRelationship(context.Context, *issuetrackerpb.CreateIssueRelationshipRequest, ...gax.CallOption) (*issuetrackerpb.IssueRelationship, error)
	ListIssueRelationships(context.Context, *issuetrackerpb.ListIssueRelationshipsRequest, ...gax.CallOption) (*issuetrackerpb.ListIssueRelationshipsResponse, error)
	ListIssueUpdates(context.Context, *issuetrackerpb.ListIssueUpdatesRequest, ...gax.CallOption) *IssueUpdateIterator
	CreateIssueComment(context.Context, *issuetrackerpb.CreateIssueCommentRequest, ...gax.CallOption) (*issuetrackerpb.IssueComment, error)
	ListIssueComments(context.Context, *issuetrackerpb.ListIssueCommentsRequest, ...gax.CallOption) *IssueCommentIterator
	UpdateIssueComment(context.Context, *issuetrackerpb.UpdateIssueCommentRequest, ...gax.CallOption) (*issuetrackerpb.IssueComment, error)
	ListAttachments(context.Context, *issuetrackerpb.ListAttachmentsRequest, ...gax.CallOption) (*issuetrackerpb.ListAttachmentsResponse, error)
	CreateHotlistEntry(context.Context, *issuetrackerpb.CreateHotlistEntryRequest, ...gax.CallOption) (*issuetrackerpb.HotlistEntry, error)
	DeleteHotlistEntry(context.Context, *issuetrackerpb.DeleteHotlistEntryRequest, ...gax.CallOption) error
	GetAutomationAccess(context.Context, *issuetrackerpb.GetAutomationAccessRequest, ...gax.CallOption) (*issuetrackerpb.GetAutomationAccessResponse, error)
}

// Client is a client for interacting with Private Issue Tracker API.
// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
//
// Manages issue and bug data, organized by a hierarchy of components.
type Client struct {
	// The internal transport-dependent client.
	internalClient internalClient

	// The call options for this service.
	CallOptions *CallOptions
}

// Wrapper methods routed to the internal client.

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *Client) Close() error {
	return c.internalClient.Close()
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *Client) setGoogleClientInfo(keyval ...string) {
	c.internalClient.setGoogleClientInfo(keyval...)
}

// Connection returns a connection to the API service.
//
// Deprecated: Connections are now pooled so this method does not always
// return the same resource.
func (c *Client) Connection() *grpc.ClientConn {
	return c.internalClient.Connection()
}

// GetComponent gets a component, including its parent hierarchy info.
func (c *Client) GetComponent(ctx context.Context, req *issuetrackerpb.GetComponentRequest, opts ...gax.CallOption) (*issuetrackerpb.Component, error) {
	return c.internalClient.GetComponent(ctx, req, opts...)
}

// ListIssues searches issues in the search cache (which may be stale), then returns the
// current state of the matched issues (which may no longer match
// ListIssuesRequest.query and may no longer be in the order indicated by
// ListIssuesRequest.order_by).
func (c *Client) ListIssues(ctx context.Context, req *issuetrackerpb.ListIssuesRequest, opts ...gax.CallOption) *IssueIterator {
	return c.internalClient.ListIssues(ctx, req, opts...)
}

// BatchGetIssues gets multiple issues with their current state by their ID. Non-existing
// issues, or issues that the caller does not have access to, are silently
// ignored. Note: The maximum number of issues that can be retrieved in one
// call is limited to 100.
func (c *Client) BatchGetIssues(ctx context.Context, req *issuetrackerpb.BatchGetIssuesRequest, opts ...gax.CallOption) (*issuetrackerpb.BatchGetIssuesResponse, error) {
	return c.internalClient.BatchGetIssues(ctx, req, opts...)
}

// GetIssue gets an issue with its current state.
func (c *Client) GetIssue(ctx context.Context, req *issuetrackerpb.GetIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	return c.internalClient.GetIssue(ctx, req, opts...)
}

// CreateIssue creates a new issue within a component, and returns the new object.
func (c *Client) CreateIssue(ctx context.Context, req *issuetrackerpb.CreateIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	return c.internalClient.CreateIssue(ctx, req, opts...)
}

// ModifyIssue updates an issue based on add and remove IssueState.  Returns the
// modified issue.
func (c *Client) ModifyIssue(ctx context.Context, req *issuetrackerpb.ModifyIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	return c.internalClient.ModifyIssue(ctx, req, opts...)
}

// CreateIssueRelationship creates a new issue relationship.
// Requires issue EDIT on the source issue and issue VIEW on the target issue.
// For relationship_type = CHILD, requires issue EDIT on the source issue and
// issue VIEW on the target issue.
func (c *Client) CreateIssueRelationship(ctx context.Context, req *issuetrackerpb.CreateIssueRelationshipRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueRelationship, error) {
	return c.internalClient.CreateIssueRelationship(ctx, req, opts...)
}

// ListIssueRelationships lists issue relationships under an issue of a type.
// Requires issue VIEW on the source issue. All target issues are included
// regardless of the caller’s issue view access. target_issue_id is always
// set. target_issue is set only if the caller has issue VIEW access to the
// target issue.
func (c *Client) ListIssueRelationships(ctx context.Context, req *issuetrackerpb.ListIssueRelationshipsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListIssueRelationshipsResponse, error) {
	return c.internalClient.ListIssueRelationships(ctx, req, opts...)
}

// ListIssueUpdates fetch a collection of IssueUpdate objects representing the change
// history of an issue, ordered by IssueUpdate.version.
func (c *Client) ListIssueUpdates(ctx context.Context, req *issuetrackerpb.ListIssueUpdatesRequest, opts ...gax.CallOption) *IssueUpdateIterator {
	return c.internalClient.ListIssueUpdates(ctx, req, opts...)
}

// CreateIssueComment creates a new issue comment in an issue
func (c *Client) CreateIssueComment(ctx context.Context, req *issuetrackerpb.CreateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	return c.internalClient.CreateIssueComment(ctx, req, opts...)
}

// ListIssueComments fetches a list of IssueComment objects.
func (c *Client) ListIssueComments(ctx context.Context, req *issuetrackerpb.ListIssueCommentsRequest, opts ...gax.CallOption) *IssueCommentIterator {
	return c.internalClient.ListIssueComments(ctx, req, opts...)
}

// UpdateIssueComment updates an issue comment.
// NB: The comment manipulation methods does not use the attachment field in
// IssueComment.
func (c *Client) UpdateIssueComment(ctx context.Context, req *issuetrackerpb.UpdateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	return c.internalClient.UpdateIssueComment(ctx, req, opts...)
}

// ListAttachments list attachments that belong to an issue. Only returns attachment metadata.
func (c *Client) ListAttachments(ctx context.Context, req *issuetrackerpb.ListAttachmentsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListAttachmentsResponse, error) {
	return c.internalClient.ListAttachments(ctx, req, opts...)
}

// CreateHotlistEntry adds an issue to a hotlist by creating a HotlistEntry. Returns the created
// HotlistEntry.
// Requires hotlist APPEND and issue VIEW permission
func (c *Client) CreateHotlistEntry(ctx context.Context, req *issuetrackerpb.CreateHotlistEntryRequest, opts ...gax.CallOption) (*issuetrackerpb.HotlistEntry, error) {
	return c.internalClient.CreateHotlistEntry(ctx, req, opts...)
}

// DeleteHotlistEntry removes an issue from a hotlist by deleting hotlistEntry. Removing an issue
// from a hotlist it does not belong to will do nothing and return.
// Requires hotlist APPEND and issue VIEW permission
func (c *Client) DeleteHotlistEntry(ctx context.Context, req *issuetrackerpb.DeleteHotlistEntryRequest, opts ...gax.CallOption) error {
	return c.internalClient.DeleteHotlistEntry(ctx, req, opts...)
}

// GetAutomationAccess checks that automation has permission on a resource.
// Does not require any permission on the resource
func (c *Client) GetAutomationAccess(ctx context.Context, req *issuetrackerpb.GetAutomationAccessRequest, opts ...gax.CallOption) (*issuetrackerpb.GetAutomationAccessResponse, error) {
	return c.internalClient.GetAutomationAccess(ctx, req, opts...)
}

// gRPCClient is a client for interacting with Private Issue Tracker API over gRPC transport.
//
// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
type gRPCClient struct {
	// Connection pool of gRPC connections to the service.
	connPool gtransport.ConnPool

	// Points back to the CallOptions field of the containing Client
	CallOptions **CallOptions

	// The gRPC API client.
	client issuetrackerpb.IssueTrackerClient

	// The x-goog-* metadata to be sent with each request.
	xGoogHeaders []string
}

// NewClient creates a new issue tracker client based on gRPC.
// The returned client must be Closed when it is done being used to clean up its underlying connections.
//
// Manages issue and bug data, organized by a hierarchy of components.
func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	clientOpts := defaultGRPCClientOptions()
	if newClientHook != nil {
		hookOpts, err := newClientHook(ctx, clientHookParams{})
		if err != nil {
			return nil, err
		}
		clientOpts = append(clientOpts, hookOpts...)
	}

	connPool, err := gtransport.DialPool(ctx, append(clientOpts, opts...)...)
	if err != nil {
		return nil, err
	}
	client := Client{CallOptions: defaultCallOptions()}

	c := &gRPCClient{
		connPool:    connPool,
		client:      issuetrackerpb.NewIssueTrackerClient(connPool),
		CallOptions: &client.CallOptions,
	}
	c.setGoogleClientInfo()

	client.internalClient = c

	return &client, nil
}

// Connection returns a connection to the API service.
//
// Deprecated: Connections are now pooled so this method does not always
// return the same resource.
func (c *gRPCClient) Connection() *grpc.ClientConn {
	return c.connPool.Conn()
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *gRPCClient) setGoogleClientInfo(keyval ...string) {
	kv := append([]string{"gl-go", gax.GoVersion}, keyval...)
	kv = append(kv, "gapic", getVersionClient(), "gax", gax.Version, "grpc", grpc.Version)
	c.xGoogHeaders = []string{"x-goog-api-client", gax.XGoogHeader(kv...)}
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *gRPCClient) Close() error {
	return c.connPool.Close()
}

// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
type restClient struct {
	// The http endpoint to connect to.
	endpoint string

	// The http client.
	httpClient *http.Client

	// The x-goog-* headers to be sent with each request.
	xGoogHeaders []string

	// Points back to the CallOptions field of the containing Client
	CallOptions **CallOptions
}

// NewRESTClient creates a new issue tracker rest client.
//
// Manages issue and bug data, organized by a hierarchy of components.
func NewRESTClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	clientOpts := append(defaultRESTClientOptions(), opts...)
	httpClient, endpoint, err := httptransport.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}

	callOpts := defaultRESTCallOptions()
	c := &restClient{
		endpoint:    endpoint,
		httpClient:  httpClient,
		CallOptions: &callOpts,
	}
	c.setGoogleClientInfo()

	return &Client{internalClient: c, CallOptions: callOpts}, nil
}

func defaultRESTClientOptions() []option.ClientOption {
	return []option.ClientOption{
		internaloption.WithDefaultEndpoint("https://placeholder-issuetracker-c2p.googleapis.com"),
		internaloption.WithDefaultMTLSEndpoint("https://placeholder-issuetracker-c2p.mtls.googleapis.com"),
		internaloption.WithDefaultAudience("https://placeholder-issuetracker-c2p.googleapis.com/"),
		internaloption.WithDefaultScopes(DefaultAuthScopes()...),
	}
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *restClient) setGoogleClientInfo(keyval ...string) {
	kv := append([]string{"gl-go", gax.GoVersion}, keyval...)
	kv = append(kv, "gapic", getVersionClient(), "gax", gax.Version, "rest", "UNKNOWN")
	c.xGoogHeaders = []string{"x-goog-api-client", gax.XGoogHeader(kv...)}
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *restClient) Close() error {
	// Replace httpClient with nil to force cleanup.
	c.httpClient = nil
	return nil
}

// Connection returns a connection to the API service.
//
// Deprecated: This method always returns nil.
func (c *restClient) Connection() *grpc.ClientConn {
	return nil
}
func (c *gRPCClient) GetComponent(ctx context.Context, req *issuetrackerpb.GetComponentRequest, opts ...gax.CallOption) (*issuetrackerpb.Component, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "component_id", req.GetComponentId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).GetComponent[0:len((*c.CallOptions).GetComponent):len((*c.CallOptions).GetComponent)], opts...)
	var resp *issuetrackerpb.Component
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.GetComponent(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ListIssues(ctx context.Context, req *issuetrackerpb.ListIssuesRequest, opts ...gax.CallOption) *IssueIterator {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).ListIssues[0:len((*c.CallOptions).ListIssues):len((*c.CallOptions).ListIssues)], opts...)
	it := &IssueIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssuesRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.Issue, string, error) {
		resp := &issuetrackerpb.ListIssuesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = c.client.ListIssues(ctx, req, settings.GRPC...)
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetIssues(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

func (c *gRPCClient) BatchGetIssues(ctx context.Context, req *issuetrackerpb.BatchGetIssuesRequest, opts ...gax.CallOption) (*issuetrackerpb.BatchGetIssuesResponse, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).BatchGetIssues[0:len((*c.CallOptions).BatchGetIssues):len((*c.CallOptions).BatchGetIssues)], opts...)
	var resp *issuetrackerpb.BatchGetIssuesResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.BatchGetIssues(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) GetIssue(ctx context.Context, req *issuetrackerpb.GetIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).GetIssue[0:len((*c.CallOptions).GetIssue):len((*c.CallOptions).GetIssue)], opts...)
	var resp *issuetrackerpb.Issue
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.GetIssue(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) CreateIssue(ctx context.Context, req *issuetrackerpb.CreateIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).CreateIssue[0:len((*c.CallOptions).CreateIssue):len((*c.CallOptions).CreateIssue)], opts...)
	var resp *issuetrackerpb.Issue
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.CreateIssue(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ModifyIssue(ctx context.Context, req *issuetrackerpb.ModifyIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ModifyIssue[0:len((*c.CallOptions).ModifyIssue):len((*c.CallOptions).ModifyIssue)], opts...)
	var resp *issuetrackerpb.Issue
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.ModifyIssue(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) CreateIssueRelationship(ctx context.Context, req *issuetrackerpb.CreateIssueRelationshipRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueRelationship, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).CreateIssueRelationship[0:len((*c.CallOptions).CreateIssueRelationship):len((*c.CallOptions).CreateIssueRelationship)], opts...)
	var resp *issuetrackerpb.IssueRelationship
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.CreateIssueRelationship(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ListIssueRelationships(ctx context.Context, req *issuetrackerpb.ListIssueRelationshipsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListIssueRelationshipsResponse, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ListIssueRelationships[0:len((*c.CallOptions).ListIssueRelationships):len((*c.CallOptions).ListIssueRelationships)], opts...)
	var resp *issuetrackerpb.ListIssueRelationshipsResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.ListIssueRelationships(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ListIssueUpdates(ctx context.Context, req *issuetrackerpb.ListIssueUpdatesRequest, opts ...gax.CallOption) *IssueUpdateIterator {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ListIssueUpdates[0:len((*c.CallOptions).ListIssueUpdates):len((*c.CallOptions).ListIssueUpdates)], opts...)
	it := &IssueUpdateIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssueUpdatesRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.IssueUpdate, string, error) {
		resp := &issuetrackerpb.ListIssueUpdatesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = c.client.ListIssueUpdates(ctx, req, settings.GRPC...)
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetIssueUpdates(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

func (c *gRPCClient) CreateIssueComment(ctx context.Context, req *issuetrackerpb.CreateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).CreateIssueComment[0:len((*c.CallOptions).CreateIssueComment):len((*c.CallOptions).CreateIssueComment)], opts...)
	var resp *issuetrackerpb.IssueComment
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.CreateIssueComment(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ListIssueComments(ctx context.Context, req *issuetrackerpb.ListIssueCommentsRequest, opts ...gax.CallOption) *IssueCommentIterator {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ListIssueComments[0:len((*c.CallOptions).ListIssueComments):len((*c.CallOptions).ListIssueComments)], opts...)
	it := &IssueCommentIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssueCommentsRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.IssueComment, string, error) {
		resp := &issuetrackerpb.ListIssueCommentsResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = c.client.ListIssueComments(ctx, req, settings.GRPC...)
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetIssueComments(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

func (c *gRPCClient) UpdateIssueComment(ctx context.Context, req *issuetrackerpb.UpdateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v&%s=%v", "issue_id", req.GetIssueId(), "comment_number", req.GetCommentNumber())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).UpdateIssueComment[0:len((*c.CallOptions).UpdateIssueComment):len((*c.CallOptions).UpdateIssueComment)], opts...)
	var resp *issuetrackerpb.IssueComment
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.UpdateIssueComment(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) ListAttachments(ctx context.Context, req *issuetrackerpb.ListAttachmentsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListAttachmentsResponse, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ListAttachments[0:len((*c.CallOptions).ListAttachments):len((*c.CallOptions).ListAttachments)], opts...)
	var resp *issuetrackerpb.ListAttachmentsResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.ListAttachments(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) CreateHotlistEntry(ctx context.Context, req *issuetrackerpb.CreateHotlistEntryRequest, opts ...gax.CallOption) (*issuetrackerpb.HotlistEntry, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "hotlist_id", req.GetHotlistId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).CreateHotlistEntry[0:len((*c.CallOptions).CreateHotlistEntry):len((*c.CallOptions).CreateHotlistEntry)], opts...)
	var resp *issuetrackerpb.HotlistEntry
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.CreateHotlistEntry(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *gRPCClient) DeleteHotlistEntry(ctx context.Context, req *issuetrackerpb.DeleteHotlistEntryRequest, opts ...gax.CallOption) error {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v&%s=%v", "hotlist_id", req.GetHotlistId(), "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).DeleteHotlistEntry[0:len((*c.CallOptions).DeleteHotlistEntry):len((*c.CallOptions).DeleteHotlistEntry)], opts...)
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		_, err = c.client.DeleteHotlistEntry(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	return err
}

func (c *gRPCClient) GetAutomationAccess(ctx context.Context, req *issuetrackerpb.GetAutomationAccessRequest, opts ...gax.CallOption) (*issuetrackerpb.GetAutomationAccessResponse, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).GetAutomationAccess[0:len((*c.CallOptions).GetAutomationAccess):len((*c.CallOptions).GetAutomationAccess)], opts...)
	var resp *issuetrackerpb.GetAutomationAccessResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = c.client.GetAutomationAccess(ctx, req, settings.GRPC...)
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetComponent gets a component, including its parent hierarchy info.
func (c *restClient) GetComponent(ctx context.Context, req *issuetrackerpb.GetComponentRequest, opts ...gax.CallOption) (*issuetrackerpb.Component, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/components/%v", req.GetComponentId())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "component_id", req.GetComponentId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).GetComponent[0:len((*c.CallOptions).GetComponent):len((*c.CallOptions).GetComponent)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.Component{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ListIssues searches issues in the search cache (which may be stale), then returns the
// current state of the matched issues (which may no longer match
// ListIssuesRequest.query and may no longer be in the order indicated by
// ListIssuesRequest.order_by).
func (c *restClient) ListIssues(ctx context.Context, req *issuetrackerpb.ListIssuesRequest, opts ...gax.CallOption) *IssueIterator {
	it := &IssueIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssuesRequest)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.Issue, string, error) {
		resp := &issuetrackerpb.ListIssuesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		baseUrl, err := url.Parse(c.endpoint)
		if err != nil {
			return nil, "", err
		}
		baseUrl.Path += fmt.Sprintf("/v1/issues")

		params := url.Values{}
		if req.GetIssueDescriptionView() != 0 {
			params.Add("issueDescriptionView", fmt.Sprintf("%v", req.GetIssueDescriptionView()))
		}
		if req.GetOrderBy() != "" {
			params.Add("orderBy", fmt.Sprintf("%v", req.GetOrderBy()))
		}
		if req.GetPageSize() != 0 {
			params.Add("pageSize", fmt.Sprintf("%v", req.GetPageSize()))
		}
		if req.GetPageToken() != "" {
			params.Add("pageToken", fmt.Sprintf("%v", req.GetPageToken()))
		}
		params.Add("query", fmt.Sprintf("%v", req.GetQuery()))
		if req.GetView() != 0 {
			params.Add("view", fmt.Sprintf("%v", req.GetView()))
		}

		baseUrl.RawQuery = params.Encode()

		// Build HTTP headers from client and context metadata.
		hds := append(c.xGoogHeaders, "Content-Type", "application/json")
		headers := gax.BuildHeaders(ctx, hds...)
		e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			if settings.Path != "" {
				baseUrl.Path = settings.Path
			}
			httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
			if err != nil {
				return err
			}
			httpReq.Header = headers

			httpRsp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return err
			}
			defer httpRsp.Body.Close()

			if err = googleapi.CheckResponse(httpRsp); err != nil {
				return err
			}

			buf, err := io.ReadAll(httpRsp.Body)
			if err != nil {
				return err
			}

			if err := unm.Unmarshal(buf, resp); err != nil {
				return err
			}

			return nil
		}, opts...)
		if e != nil {
			return nil, "", e
		}
		it.Response = resp
		return resp.GetIssues(), resp.GetNextPageToken(), nil
	}

	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

// BatchGetIssues gets multiple issues with their current state by their ID. Non-existing
// issues, or issues that the caller does not have access to, are silently
// ignored. Note: The maximum number of issues that can be retrieved in one
// call is limited to 100.
func (c *restClient) BatchGetIssues(ctx context.Context, req *issuetrackerpb.BatchGetIssuesRequest, opts ...gax.CallOption) (*issuetrackerpb.BatchGetIssuesResponse, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues:batchGet")

	params := url.Values{}
	if req.GetIssueDescriptionView() != 0 {
		params.Add("issueDescriptionView", fmt.Sprintf("%v", req.GetIssueDescriptionView()))
	}
	if items := req.GetIssueIds(); len(items) > 0 {
		for _, item := range items {
			params.Add("issueIds", fmt.Sprintf("%v", item))
		}
	}
	if req.GetView() != 0 {
		params.Add("view", fmt.Sprintf("%v", req.GetView()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := append(c.xGoogHeaders, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).BatchGetIssues[0:len((*c.CallOptions).BatchGetIssues):len((*c.CallOptions).BatchGetIssues)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.BatchGetIssuesResponse{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// GetIssue gets an issue with its current state.
func (c *restClient) GetIssue(ctx context.Context, req *issuetrackerpb.GetIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v", req.GetIssueId())

	params := url.Values{}
	if req.GetIssueDescriptionView() != 0 {
		params.Add("issueDescriptionView", fmt.Sprintf("%v", req.GetIssueDescriptionView()))
	}
	if req.GetView() != 0 {
		params.Add("view", fmt.Sprintf("%v", req.GetView()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).GetIssue[0:len((*c.CallOptions).GetIssue):len((*c.CallOptions).GetIssue)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.Issue{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// CreateIssue creates a new issue within a component, and returns the new object.
func (c *restClient) CreateIssue(ctx context.Context, req *issuetrackerpb.CreateIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	body := req.GetIssue()
	jsonReq, err := m.Marshal(body)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues")

	params := url.Values{}
	if req.GetTemplateOptions().GetApplyTemplate() {
		params.Add("templateOptions.applyTemplate", fmt.Sprintf("%v", req.GetTemplateOptions().GetApplyTemplate()))
	}
	if req.GetTemplateOptions().GetTemplateId() != 0 {
		params.Add("templateOptions.templateId", fmt.Sprintf("%v", req.GetTemplateOptions().GetTemplateId()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := append(c.xGoogHeaders, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).CreateIssue[0:len((*c.CallOptions).CreateIssue):len((*c.CallOptions).CreateIssue)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.Issue{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("POST", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ModifyIssue updates an issue based on add and remove IssueState.  Returns the
// modified issue.
func (c *restClient) ModifyIssue(ctx context.Context, req *issuetrackerpb.ModifyIssueRequest, opts ...gax.CallOption) (*issuetrackerpb.Issue, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	jsonReq, err := m.Marshal(req)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v:modify", req.GetIssueId())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).ModifyIssue[0:len((*c.CallOptions).ModifyIssue):len((*c.CallOptions).ModifyIssue)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.Issue{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("POST", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// CreateIssueRelationship creates a new issue relationship.
// Requires issue EDIT on the source issue and issue VIEW on the target issue.
// For relationship_type = CHILD, requires issue EDIT on the source issue and
// issue VIEW on the target issue.
func (c *restClient) CreateIssueRelationship(ctx context.Context, req *issuetrackerpb.CreateIssueRelationshipRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueRelationship, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	body := req.GetIssueRelationship()
	jsonReq, err := m.Marshal(body)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v/relationships", req.GetIssueId())

	params := url.Values{}
	if req.GetRelationshipType() != 0 {
		params.Add("relationshipType", fmt.Sprintf("%v", req.GetRelationshipType()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).CreateIssueRelationship[0:len((*c.CallOptions).CreateIssueRelationship):len((*c.CallOptions).CreateIssueRelationship)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.IssueRelationship{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("POST", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ListIssueRelationships lists issue relationships under an issue of a type.
// Requires issue VIEW on the source issue. All target issues are included
// regardless of the caller’s issue view access. target_issue_id is always
// set. target_issue is set only if the caller has issue VIEW access to the
// target issue.
func (c *restClient) ListIssueRelationships(ctx context.Context, req *issuetrackerpb.ListIssueRelationshipsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListIssueRelationshipsResponse, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v/relationships", req.GetIssueId())

	params := url.Values{}
	if req.GetIssueDescriptionView() != 0 {
		params.Add("issueDescriptionView", fmt.Sprintf("%v", req.GetIssueDescriptionView()))
	}
	if req.GetQuery() != "" {
		params.Add("query", fmt.Sprintf("%v", req.GetQuery()))
	}
	if req.GetRelationshipType() != 0 {
		params.Add("relationshipType", fmt.Sprintf("%v", req.GetRelationshipType()))
	}
	if req.GetView() != 0 {
		params.Add("view", fmt.Sprintf("%v", req.GetView()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).ListIssueRelationships[0:len((*c.CallOptions).ListIssueRelationships):len((*c.CallOptions).ListIssueRelationships)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.ListIssueRelationshipsResponse{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ListIssueUpdates fetch a collection of IssueUpdate objects representing the change
// history of an issue, ordered by IssueUpdate.version.
func (c *restClient) ListIssueUpdates(ctx context.Context, req *issuetrackerpb.ListIssueUpdatesRequest, opts ...gax.CallOption) *IssueUpdateIterator {
	it := &IssueUpdateIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssueUpdatesRequest)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.IssueUpdate, string, error) {
		resp := &issuetrackerpb.ListIssueUpdatesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		baseUrl, err := url.Parse(c.endpoint)
		if err != nil {
			return nil, "", err
		}
		baseUrl.Path += fmt.Sprintf("/v1/issues/%v/issueUpdates", req.GetIssueId())

		params := url.Values{}
		if req.GetIssueCommentView() != 0 {
			params.Add("issueCommentView", fmt.Sprintf("%v", req.GetIssueCommentView()))
		}
		if req.GetPageSize() != 0 {
			params.Add("pageSize", fmt.Sprintf("%v", req.GetPageSize()))
		}
		if req.GetPageToken() != "" {
			params.Add("pageToken", fmt.Sprintf("%v", req.GetPageToken()))
		}
		if req.GetSortBy() != "" {
			params.Add("sortBy", fmt.Sprintf("%v", req.GetSortBy()))
		}

		baseUrl.RawQuery = params.Encode()

		// Build HTTP headers from client and context metadata.
		hds := append(c.xGoogHeaders, "Content-Type", "application/json")
		headers := gax.BuildHeaders(ctx, hds...)
		e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			if settings.Path != "" {
				baseUrl.Path = settings.Path
			}
			httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
			if err != nil {
				return err
			}
			httpReq.Header = headers

			httpRsp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return err
			}
			defer httpRsp.Body.Close()

			if err = googleapi.CheckResponse(httpRsp); err != nil {
				return err
			}

			buf, err := io.ReadAll(httpRsp.Body)
			if err != nil {
				return err
			}

			if err := unm.Unmarshal(buf, resp); err != nil {
				return err
			}

			return nil
		}, opts...)
		if e != nil {
			return nil, "", e
		}
		it.Response = resp
		return resp.GetIssueUpdates(), resp.GetNextPageToken(), nil
	}

	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

// CreateIssueComment creates a new issue comment in an issue
func (c *restClient) CreateIssueComment(ctx context.Context, req *issuetrackerpb.CreateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	body := req.GetComment()
	jsonReq, err := m.Marshal(body)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v/comments", req.GetIssueId())

	params := url.Values{}
	if req.GetSignificanceOverride() != 0 {
		params.Add("significanceOverride", fmt.Sprintf("%v", req.GetSignificanceOverride()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).CreateIssueComment[0:len((*c.CallOptions).CreateIssueComment):len((*c.CallOptions).CreateIssueComment)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.IssueComment{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("POST", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ListIssueComments fetches a list of IssueComment objects.
func (c *restClient) ListIssueComments(ctx context.Context, req *issuetrackerpb.ListIssueCommentsRequest, opts ...gax.CallOption) *IssueCommentIterator {
	it := &IssueCommentIterator{}
	req = proto.Clone(req).(*issuetrackerpb.ListIssueCommentsRequest)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	it.InternalFetch = func(pageSize int, pageToken string) ([]*issuetrackerpb.IssueComment, string, error) {
		resp := &issuetrackerpb.ListIssueCommentsResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		baseUrl, err := url.Parse(c.endpoint)
		if err != nil {
			return nil, "", err
		}
		baseUrl.Path += fmt.Sprintf("/v1/issues/%v/comments", req.GetIssueId())

		params := url.Values{}
		if req.GetIssueCommentView() != 0 {
			params.Add("issueCommentView", fmt.Sprintf("%v", req.GetIssueCommentView()))
		}
		if req.GetPageSize() != 0 {
			params.Add("pageSize", fmt.Sprintf("%v", req.GetPageSize()))
		}
		if req.GetPageToken() != "" {
			params.Add("pageToken", fmt.Sprintf("%v", req.GetPageToken()))
		}
		if req.GetSortBy() != "" {
			params.Add("sortBy", fmt.Sprintf("%v", req.GetSortBy()))
		}

		baseUrl.RawQuery = params.Encode()

		// Build HTTP headers from client and context metadata.
		hds := append(c.xGoogHeaders, "Content-Type", "application/json")
		headers := gax.BuildHeaders(ctx, hds...)
		e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			if settings.Path != "" {
				baseUrl.Path = settings.Path
			}
			httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
			if err != nil {
				return err
			}
			httpReq.Header = headers

			httpRsp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return err
			}
			defer httpRsp.Body.Close()

			if err = googleapi.CheckResponse(httpRsp); err != nil {
				return err
			}

			buf, err := io.ReadAll(httpRsp.Body)
			if err != nil {
				return err
			}

			if err := unm.Unmarshal(buf, resp); err != nil {
				return err
			}

			return nil
		}, opts...)
		if e != nil {
			return nil, "", e
		}
		it.Response = resp
		return resp.GetIssueComments(), resp.GetNextPageToken(), nil
	}

	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

// UpdateIssueComment updates an issue comment.
// NB: The comment manipulation methods does not use the attachment field in
// IssueComment.
func (c *restClient) UpdateIssueComment(ctx context.Context, req *issuetrackerpb.UpdateIssueCommentRequest, opts ...gax.CallOption) (*issuetrackerpb.IssueComment, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	body := req.GetComment()
	jsonReq, err := m.Marshal(body)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v/comments/%v", req.GetIssueId(), req.GetCommentNumber())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v&%s=%v", "issue_id", req.GetIssueId(), "comment_number", req.GetCommentNumber())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).UpdateIssueComment[0:len((*c.CallOptions).UpdateIssueComment):len((*c.CallOptions).UpdateIssueComment)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.IssueComment{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("PUT", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// ListAttachments list attachments that belong to an issue. Only returns attachment metadata.
func (c *restClient) ListAttachments(ctx context.Context, req *issuetrackerpb.ListAttachmentsRequest, opts ...gax.CallOption) (*issuetrackerpb.ListAttachmentsResponse, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/issues/%v/attachments", req.GetIssueId())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).ListAttachments[0:len((*c.CallOptions).ListAttachments):len((*c.CallOptions).ListAttachments)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.ListAttachmentsResponse{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// CreateHotlistEntry adds an issue to a hotlist by creating a HotlistEntry. Returns the created
// HotlistEntry.
// Requires hotlist APPEND and issue VIEW permission
func (c *restClient) CreateHotlistEntry(ctx context.Context, req *issuetrackerpb.CreateHotlistEntryRequest, opts ...gax.CallOption) (*issuetrackerpb.HotlistEntry, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	jsonReq, err := m.Marshal(req)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v1/hotlists/%v/entries", req.GetHotlistId())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "hotlist_id", req.GetHotlistId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).CreateHotlistEntry[0:len((*c.CallOptions).CreateHotlistEntry):len((*c.CallOptions).CreateHotlistEntry)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.HotlistEntry{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("PUT", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// DeleteHotlistEntry removes an issue from a hotlist by deleting hotlistEntry. Removing an issue
// from a hotlist it does not belong to will do nothing and return.
// Requires hotlist APPEND and issue VIEW permission
func (c *restClient) DeleteHotlistEntry(ctx context.Context, req *issuetrackerpb.DeleteHotlistEntryRequest, opts ...gax.CallOption) error {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}
	baseUrl.Path += fmt.Sprintf("/v1/hotlists/%v/entries/%v", req.GetHotlistId(), req.GetIssueId())

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v&%s=%v", "hotlist_id", req.GetHotlistId(), "issue_id", req.GetIssueId())}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	return gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("DELETE", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		// Returns nil if there is no error, otherwise wraps
		// the response code and body into a non-nil error
		return googleapi.CheckResponse(httpRsp)
	}, opts...)
}

// GetAutomationAccess checks that automation has permission on a resource.
// Does not require any permission on the resource
func (c *restClient) GetAutomationAccess(ctx context.Context, req *issuetrackerpb.GetAutomationAccessRequest, opts ...gax.CallOption) (*issuetrackerpb.GetAutomationAccessResponse, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("")

	params := url.Values{}
	if req.GetRelation() != "" {
		params.Add("relation", fmt.Sprintf("%v", req.GetRelation()))
	}
	if req.GetResourceName() != "" {
		params.Add("resourceName", fmt.Sprintf("%v", req.GetResourceName()))
	}
	if req.GetUser().GetEmailAddress() != "" {
		params.Add("user.emailAddress", fmt.Sprintf("%v", req.GetUser().GetEmailAddress()))
	}
	if req.GetUser().GetObfuscatedEmailAddress() != "" {
		params.Add("user.obfuscatedEmailAddress", fmt.Sprintf("%v", req.GetUser().GetObfuscatedEmailAddress()))
	}

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := append(c.xGoogHeaders, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).GetAutomationAccess[0:len((*c.CallOptions).GetAutomationAccess):len((*c.CallOptions).GetAutomationAccess)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &issuetrackerpb.GetAutomationAccessResponse{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		httpRsp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer httpRsp.Body.Close()

		if err = googleapi.CheckResponse(httpRsp); err != nil {
			return err
		}

		buf, err := io.ReadAll(httpRsp.Body)
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}
