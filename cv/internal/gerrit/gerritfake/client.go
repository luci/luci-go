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
	"fmt"

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
func (c *Client) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	c.f.m.Lock()
	defer c.f.m.Unlock()

	ch, found := c.f.cs[key(c.host, int(in.GetNumber()))]
	if !found {
		return nil, status.Errorf(codes.NotFound, "change %s/%d not found", c.host, in.GetNumber())
	}
	s := ch.ACLs(OpRead, c.luciProject)
	if s.Code() != codes.OK {
		return nil, s.Err()
	}
	ps, err := ch.resolveRevisionToPS(in.GetRevisionId())
	if err != nil {
		return nil, err
	}
	start := relkey(c.host, int(in.GetNumber()), ps)

	// NOTE: Gerrit actually guarantees specific order.  For simplicity, this fake
	// doesn't. We just recurse in direction of both child->parent and
	// parent->child and add visited changes to the list.
	res := &gerritpb.GetRelatedChangesResponse{}
	added := stringset.New(10)
	add := func(cc *gerritpb.GetRelatedChangesResponse_ChangeAndCommit) {
		if added.Add(fmt.Sprintf("%d/%d", cc.GetNumber(), cc.GetPatchset())) {
			res.Changes = append(res.Changes, cc)
		}
	}

	var visit func(string, func(string) []string) error
	visit = func(relkey string, next func(string) []string) error {
		ch, rev, ri, err := c.f.resolveRelkeyLocked(relkey)
		if err != nil {
			return err
		}
		cc := &gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
			Project:         ch.Info.GetProject(),
			Number:          ch.Info.GetNumber(),
			Patchset:        int64(ri.GetNumber()),
			CurrentPatchset: int64(ch.Info.GetRevisions()[ch.Info.GetCurrentRevision()].GetNumber()),
			Commit:          &gerritpb.CommitInfo{Id: rev},
		}
		for _, prelkey := range c.f.parentOf[relkey] {
			_, parentRev, _, err := c.f.resolveRelkeyLocked(prelkey)
			if err != nil {
				return err
			}
			cc.GetCommit().Parents = append(cc.GetCommit().Parents, &gerritpb.CommitInfo_Parent{Id: parentRev})
		}
		add(cc)
		for _, c := range next(relkey) {
			if err := visit(c, next); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(start, func(rk string) []string { return c.f.childOf[rk] }); err != nil {
		return nil, err
	}
	if err := visit(start, func(rk string) []string { return c.f.parentOf[rk] }); err != nil {
		return nil, err
	}
	if len(res.GetChanges()) == 1 {
		// Since the start change is always first by construction, result contains
		// only the start change. Do what Gerrit does and return empty result.
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
