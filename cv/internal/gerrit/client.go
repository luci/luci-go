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

	// Returns the email addresses linked in the given Gerrit account.
	//
	// https://gerrit-review.googlesource.com/Documentation/rest-api-accounts.html#list-account-emails
	ListAccountEmails(ctx context.Context, req *gerritpb.ListAccountEmailsRequest, opts ...grpc.CallOption) (*gerritpb.ListAccountEmailsResponse, error)
}

// Factory creates Client tied to Gerrit host and LUCI project.
//
// Gerrit host and LUCI project determine the authentication being used.
type Factory interface {
	MakeClient(ctx context.Context, gerritHost, luciProject string) (Client, error)
	MakeMirrorIterator(ctx context.Context) *MirrorIterator
}

// NewFactory returns Factory for use in production.
func NewFactory(ctx context.Context, mirrorHostPrefixes ...string) (Factory, error) {
	f, err := newProd(ctx, mirrorHostPrefixes...)
	if err != nil {
		return nil, err
	}
	return CachingFactory(64, InstrumentedFactory(TimeLimitedFactory(f))), nil
}
