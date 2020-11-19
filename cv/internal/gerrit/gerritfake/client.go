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

package gerritfake

import (
	"context"

	"google.golang.org/grpc"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/gerrit"
)

// Client implements client for Fake Gerrit.
type Client struct {
	f           *Fake
	luciProject string // used in ACL checks.
}

var _ gerrit.Client = (*Client)(nil)

///////////////////////////////////////////////////////////////////////////////
// Read RPCs

// Loads a change by id.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-change
func (c *Client) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic("not implemented") // TODO: Implement. Must return a copy.
}

// Gets Mergeable status for a change.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-projects.html#get-mergeable-info
func (c *Client) GetMergeable(ctx context.Context, in *gerritpb.GetMergeableRequest, opts ...grpc.CallOption) (*gerritpb.MergeableInfo, error) {
	panic("not implemented") // TODO: Implement
}

// Lists the files that were modified, added or deleted in a revision.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-files
func (c *Client) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	panic("not implemented") // TODO: Implement
}
