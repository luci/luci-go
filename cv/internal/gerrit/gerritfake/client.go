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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/gerrit"
)

// Client implements client for Fake Gerrit.
type Client struct {
	f           *Fake
	luciProject string // used in ACL checks.
	host        string
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

// Retrieves related changes of a revision.
//
// Related changes are changes that either depend on, or are dependencies of
// the revision.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-related-changes
func (client *Client) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	client.f.m.Lock()
	defer client.f.m.Unlock()

	change, found := client.f.cs[key(client.host, int(in.GetNumber()))]
	if !found {
		return nil, status.Errorf(codes.NotFound, "change %s/%d not found", client.host, in.GetNumber())
	}
	if status := change.ACLs(OpRead, client.luciProject); status.Code() != codes.OK {
		return nil, status.Err()
	}
	ps, _, err := change.resolveRevision(in.GetRevisionId())
	if err != nil {
		return nil, err
	}
	start := psKey(client.host, int(in.GetNumber()), ps)

	res := &gerritpb.GetRelatedChangesResponse{}
	added := stringset.New(10)
	add := func(psk string) error {
		if !added.Add(psk) {
			return nil
		}
		change, rev, ri, err := client.f.resolvePSKeyLocked(psk)
		if err != nil {
			return err
		}
		cc := &gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
			Project:         change.Info.GetProject(),
			Number:          change.Info.GetNumber(),
			Patchset:        int64(ri.GetNumber()),
			CurrentPatchset: int64(change.Info.GetRevisions()[change.Info.GetCurrentRevision()].GetNumber()),
			Commit:          &gerritpb.CommitInfo{Id: rev},
		}
		for _, parentPSkey := range client.f.parentsOf[psk] {
			_, parentRev, _, err := client.f.resolvePSKeyLocked(parentPSkey)
			if err != nil {
				return err
			}
			cc.GetCommit().Parents = append(cc.GetCommit().Parents, &gerritpb.CommitInfo_Parent{Id: parentRev})
		}
		res.Changes = append(res.Changes, cc)
		return nil
	}
	// NOTE: Gerrit actually guarantees specific order. For simplicity, this fake
	// doesn't. We just recurse in direction of both child->parent and
	// parent->child and add visited changes to the list.
	if err := visitNodesDFS(start, client.f.childrenOf, add); err != nil {
		return nil, err
	}
	if err := visitNodesDFS(start, client.f.parentsOf, add); err != nil {
		return nil, err
	}
	if len(res.GetChanges()) == 1 {
		// Just the starting change itself, emulate Gerrit by returning empty list.
		res.Changes = nil
	}
	return res, nil
}

// Lists the files that were modified, added or deleted in a revision.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-files
func (c *Client) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	panic("not implemented") // TODO: Implement
}

// visitNodesDFS visits all nodes reachable from the current node via depth
// first search.
//
// Calls clbk for each node visited. If clbk returns error, visitNodesDFS aborts
// immediatey and returns the same error.
func visitNodesDFS(node string, edges map[string][]string, clbk func(node string) error) error {
	visited := stringset.New(1)

	var visit func(n string) error
	visit = func(n string) error {
		if !visited.Add(n) {
			return nil
		}
		for _, m := range edges[n] {
			if err := visit(m); err != nil {
				return err
			}
		}
		return clbk(n)
	}
	return visit(node)
}
