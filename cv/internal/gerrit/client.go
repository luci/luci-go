// Copyright 2020 The LUCI Authors.
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

package gerrit

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// Client defines a subset of Gerrit API used by CV.
type Client interface {
	// Lists changes that match a query.
	//
	// Note, although the Gerrit API supports multiple queries, for which
	// it can return multiple lists of changes, this is not a foreseen use-case
	// so this API just includes one query with one returned list of changes.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
	ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error)

	// Loads a change by id.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-change
	GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error)

	// Retrieves related changes of a revision.
	//
	// Related changes are changes that either depend on, or are dependencies of
	// the revision.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-related-changes
	GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error)

	// Lists the files that were modified, added or deleted in a revision.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-files
	ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error)

	// Set various review bits on a change.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#set-review
	SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error)

	// Submit a specific revision of a change.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#submit-revision
	SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error)
}

// Client must be a subset of gerritpb.Client.
var _ Client = (gerritpb.GerritClient)(nil)

var clientCtxKey = "go.chromium.org/luci/cv/internal/gerrit.Client"

// UseClientFactory puts a given ClientFactory into in the context.
func UseClientFactory(ctx context.Context, f ClientFactory) context.Context {
	return context.WithValue(ctx, &clientCtxKey, f)
}

// UseProd puts a production ClientFactory into in the context.
func UseProd(ctx context.Context) (context.Context, error) {
	f, err := newFactory(ctx)
	if err != nil {
		return nil, err
	}
	return UseClientFactory(ctx, f.makeClient), nil
}

// CurrentClient returns the Client in the context or an error.
func CurrentClient(ctx context.Context, gerritHost, luciProject string) (Client, error) {
	f, _ := ctx.Value(&clientCtxKey).(ClientFactory)
	if f == nil {
		return nil, errors.New("not a valid Gerrit context, no ClientFactory available")
	}
	return f(ctx, gerritHost, luciProject)
}

// ClientFactory creates Client tied to Gerrit host and LUCI project.
//
// Gerrit host and LUCI project determine the authentication being used.
type ClientFactory func(ctx context.Context, gerritHost, luciProject string) (Client, error)
