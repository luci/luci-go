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
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
)

// PagingListChanges is a wrapper around Gerrit.ListChanges RPC that correctly
// "pages" through responses.
//
// Unlike typical but incorrect paging via ListChangesRequest.Offset argument,
// this function correctly fetches all changes up to a desired limit relying on
// undocumented ordering of changes in the ListChanges response by descending
// ChangeInfo.Updated timestamp.
//
// ListChangesRequest.Limit and ListChangesRequest.Offset must not be set.
//
// ListChangesRequest.Query may contain before: until: since: after: predicates,
// but it's more efficient to specify those via
// PagingListChangesOptions UpdatedAfter and UpdatedBefore options.
//
// On error, also returns partial results fetched so far and .MoreChanges=true.
func PagingListChanges(ctx context.Context, client PagingListChangesClient,
	req *gerritpb.ListChangesRequest, opts PagingListChangesOptions,
	grpcOpts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	// This function is not tested. Keep it as small as possible.
	p := listChangesPager{
		client:   client,
		req:      req,
		opts:     opts,
		grpcOpts: grpcOpts,
	}
	return p.pagingListChanges(ctx)
}

// PagingListChangesClient defines what PageListChanges actually needs from
// Gerrit client.
type PagingListChangesClient interface {
	ListChanges(context.Context, *gerritpb.ListChangesRequest, ...grpc.CallOption) (*gerritpb.ListChangesResponse, error)
}

// PagingListChangesOptions customizes behavior of PagingListChanges function.
type PagingListChangesOptions struct {
	// Limit limits how many changes to fetch in total. Required. Must be >0.
	Limit int

	// PageSize limits how many change to fetch per RPC request.
	// Defaults to 100. Maximum allowed is 1000.
	PageSize int

	// MoreChangesTrustFactor limits trust in MoreChanges=false from a
	// Gerrit RPC response to only when RPC returned fewer changes
	// than PageSize*MoreChangesTrustFactor.
	//
	// Must be in (0, 1] range. 1 means always trust. Defaults to 0.9.
	//
	// Lower value is recommended when a query matches many changes which are not
	// visible to the account making the request. Due to internal Gerrit
	// implementation quirk, in such cases .MoreChanges=false can be set
	// incorrectly, meaning even when there are more results than requested.
	//
	// For certainty, set this to 0.001. In the worst case, it'd result in one
	// additional ListChanges RPC.
	MoreChangesTrustFactor float64

	// The following 2 parameters are optional. They are here to avoid duplicate
	// `after:` and `before:` query predicates in resulting queries to Gerrit,
	// without having to parse user-provided query string.

	// UpdatedAfter can be used to limit which changes the query should cover.
	// NOTE: it is inclusive, just like Gerrit's `after:` query predicate.
	// Defaults to no restriction.
	UpdatedAfter time.Time
	// UpdatedBefore can be used to limit which changes the query should cover.
	// NOTE: it is inclusive, just like Gerrit's `before:` query predicate.
	// Defaults to no restriction.
	UpdatedBefore time.Time
}

type listChangesPager struct {
	client   PagingListChangesClient
	req      *gerritpb.ListChangesRequest
	opts     PagingListChangesOptions
	grpcOpts []grpc.CallOption

	queryBuilder strings.Builder
}

// pagingListChanges actually implements PagingListChanges.
//
// It's actually tested by mocking `client` in listChangesPager.
func (p *listChangesPager) pagingListChanges(ctx context.Context) (*gerritpb.ListChangesResponse, error) {
	switch {
	case p.opts.Limit <= 0:
		return nil, errors.New("PagingListChangesOptions.Limit must positive")
	case p.opts.PageSize < 0 || p.opts.PageSize > 1000:
		return nil, errors.New("PagingListChangesOptions.PageSize must be in [0 ... 1000]")
	case p.opts.MoreChangesTrustFactor < 0 || p.opts.MoreChangesTrustFactor > 1:
		return nil, errors.New("PagingListChangesOptions.TrustMoreChangesFactor must be in (0 ... 1.0]")

	case p.req.GetLimit() != 0:
		return nil, errors.New("ListChangesRequest.Limit must not be set")
	case p.req.GetOffset() != 0:
		return nil, errors.New("ListChangesRequest.Offset must not be set")
	}

	if p.opts.PageSize == 0 {
		p.opts.PageSize = 100
	}
	if p.opts.MoreChangesTrustFactor == 0 {
		p.opts.MoreChangesTrustFactor = 0.9
	}

	changes, more, err := p.fetch(ctx)
	return &gerritpb.ListChangesResponse{
		Changes:     changes,
		MoreChanges: more,
	}, err
}

// fetch does actual paging to fetch results.
//
// Like PagingListChanges, it may return partial results with non-nil error.
func (p *listChangesPager) fetch(ctx context.Context) ([]*gerritpb.ChangeInfo, bool, error) {
	changes, more, err := p.doRPC(ctx, p.prepareRequest(p.opts.UpdatedAfter, p.opts.UpdatedBefore))
	switch {
	case err != nil:
		return nil, true, err
	case len(changes) > p.opts.Limit:
		return changes[:p.opts.Limit], true, nil
	case !more:
		return changes, false, nil
	case len(changes) == 0:
		panic("doRPC ensures 0 changes imply !more")
	}

	// Continue fetching older less recently updated changes until we get the
	// required number.
	// Use inclusive `before:` least recently updated to avoid missing any changes
	// with the same updated timestamp. This will likely (% see possibility below)
	// produce overlap with already fetched changes.
	// However, we may still miss some changes if a change got updated between our
	// RPCs. This is taken care of at the end.

	deduper := listChangesDeduper{}
	for len(changes) <= p.opts.Limit && more {
		oldest := changes[len(changes)-1]
		req := p.prepareRequest(p.opts.UpdatedAfter, oldest.GetUpdated().AsTime())
		var olderChanges []*gerritpb.ChangeInfo
		if olderChanges, more, err = p.doRPC(ctx, req); err != nil {
			return changes, true, err
		}
		switch newChanges := deduper.appendSorted(changes, olderChanges); {
		case more && len(newChanges) == len(changes):
			// No progress, likely because there are >opts.PageSize Gerrit changes
			// with the same .updated timestamp.
			return changes, true, errors.Fmt("PagingListChanges stuck on %v: no new changes out of %d fetched", req,
				len(olderChanges))
		default:
			changes = newChanges
		}
	}

	// Either enough changes already fetched or there are no more changes.
	// Still need to ensure no change was missed due to concurrent update.
	newest := changes[0]
	newerChanges, newMore, err := p.doRPC(ctx,
		p.prepareRequest(newest.GetUpdated().AsTime(), p.opts.UpdatedBefore))
	switch {
	case err != nil:
		return changes, true, err
	case newMore:
		// It's possible to recurse here into fetchInner but with
		// p.opts.UpdatedAfter == newest.GetUpdated().AsTime().
		// However, this is still not guaranteed to catch up with rate of changes,
		// and in practice shouldn't be necessary.
		return changes, true,

			transient.Tag.Apply(errors.New("PagingListChanges can't keep up with the rate of updates to changes. " +
				"Try increasing PagingListChangesOptions.PageSize or " +
				"restricting PagingListChangesOptions.UpdatedBefore to past timestamp"))
	}
	changes = deduper.mergeSorted(newerChanges, changes)
	if len(changes) > p.opts.Limit {
		changes = changes[:p.opts.Limit]
		more = true
	}
	return changes, more, nil
}

func (p *listChangesPager) prepareRequest(after, before time.Time) *gerritpb.ListChangesRequest {
	req := proto.Clone(p.req).(*gerritpb.ListChangesRequest)
	p.queryBuilder.Reset()
	p.queryBuilder.WriteString(req.GetQuery())
	if !after.IsZero() {
		p.queryBuilder.WriteRune(' ')
		p.queryBuilder.WriteString("after:")
		p.queryBuilder.WriteString(FormatTime(after))
	}
	if !before.IsZero() {
		p.queryBuilder.WriteRune(' ')
		p.queryBuilder.WriteString("before:")
		p.queryBuilder.WriteString(FormatTime(before))
	}
	req.Query = p.queryBuilder.String()
	req.Limit = int64(p.opts.PageSize)
	return req
}

// doRPC executes one RPC and validates its response.
//
// Notably, moreChanges is false if and only if it is trusted.
func (p *listChangesPager) doRPC(ctx context.Context, req *gerritpb.ListChangesRequest) (
	[]*gerritpb.ChangeInfo, bool, error) {
	resp, err := p.client.ListChanges(ctx, req, p.grpcOpts...)
	if err != nil {
		return nil, false, err
	}
	if err := p.verifyCanonicalOrder(ctx, resp.GetChanges()); err != nil {
		return nil, false, err
	}

	trustMax := int(float64(req.GetLimit()) * p.opts.MoreChangesTrustFactor)
	switch l := len(resp.GetChanges()); {
	case resp.GetMoreChanges() && l == 0:
		return nil, false, errors.Fmt("Broken ListChanges(Limit=%d) response with 0 changes yet MoreChanges=true", req.GetLimit())
	case l > trustMax:
		// Assume there are more changes regardless of the actual MoreChanges value.
		return resp.GetChanges(), true, nil
	case resp.GetMoreChanges():
		logging.Warningf(ctx, "Unexpected ListChanges(limit=%d) gave %d changes yet MoreChanges=true",
			p.opts.Limit, l)
		return resp.GetChanges(), true, nil
	default:
		return resp.GetChanges(), false, nil
	}
}

// verifyCanonicalOrder ensures changes are sorted by descending updated
// timestamp.
func (p *listChangesPager) verifyCanonicalOrder(ctx context.Context, cs []*gerritpb.ChangeInfo) error {
	for i := len(cs) - 1; i > 0; i-- {
		if cs[i].GetUpdated().AsTime().After(cs[i-1].GetUpdated().AsTime()) {
			logging.Errorf(ctx,
				"CRITICAL: PagingListChanges is no longer correct. Gerrit returned "+
					"ListChangesResponse.Changes not ordered by updated timestamp: %d@%s, %d@%s "+
					"Query used %q",
				cs[i-1].GetNumber(), cs[i-1].GetUpdated().AsTime(),
				cs[i].GetNumber(), cs[i].GetUpdated().AsTime(),
				p.req.GetQuery())
			return errors.New("ListChangesResponse.Changes not ordered by updated timestamp")
		}
	}
	return nil
}

// listChangesDeduper efficiently combines output of several ListChanges RPCs.
//
// de-duplication is done by Change Number, since all changes must come from the
// same Gerrit host.
type listChangesDeduper struct {
	seenIds []int64
}

// appendSorted appends `add` to `dest`, both must be in the canonical order.
//
// If `dest` last change (i.e. least recently modified) has the same updated
// timestamp T as first (i.e. most recently modified) to the `add`,
// then appendSorted skips duplicate changes from `add` with the updated
// timestamp T.
//
// Doesn't check nor guarantee that resulting slice is free of duplicated
// changes.
func (d *listChangesDeduper) appendSorted(dest, add []*gerritpb.ChangeInfo) []*gerritpb.ChangeInfo {
	if len(dest) == 0 {
		return append(dest, add...)
	}
	d.seenIds = d.seenIds[:0] // clear
	u := dest[len(dest)-1].GetUpdated().AsTime()
	// Use slices instead of maps because typical case is 1 change overlap.
	// In the worst case, it'll be O(len(changes)^2) == O(pageSize^2) with
	// pageSize max of 1000.
	for i := len(dest) - 1; i >= 0 && dest[i].GetUpdated().AsTime() == u; i-- {
		d.seenIds = append(d.seenIds, dest[i].GetNumber())
	}
	for i, c := range add {
		if c.GetUpdated().AsTime() != u {
			return append(dest, add[i:]...)
		}
		skip := false
		for _, id := range d.seenIds {
			if id == c.GetNumber() {
				skip = true
				break
			}
		}
		if !skip {
			dest = append(dest, c)
		}
	}
	return dest
}

// mergeSorted merges two slice in the canonical order and removes duplicates,
// keeping the most recently updated version of the change.
func (d *listChangesDeduper) mergeSorted(a, b []*gerritpb.ChangeInfo) []*gerritpb.ChangeInfo {
	// We expect very few duplicates.
	c := make([]*gerritpb.ChangeInfo, 0, len(a)+len(b))
	m := make(map[int64]struct{}, len(c))
	for len(a)+len(b) > 0 {
		var ch *gerritpb.ChangeInfo
		switch {
		case len(a) == 0:
			ch, b = b[0], b[1:]
		case len(b) == 0:
			ch, a = a[0], a[1:]
		default:
			switch au, bu := a[0].GetUpdated().AsTime(), b[0].GetUpdated().AsTime(); {
			case au.After(bu):
				ch, a = a[0], a[1:]
			case bu.After(au):
				ch, b = b[0], b[1:]
			// Arbitrary, but deterministically, disambiguate on change number.
			case a[0].GetNumber() < b[0].GetNumber():
				ch, a = a[0], a[1:]
			default:
				ch, b = b[0], b[1:]
			}
		}
		if _, dup := m[ch.GetNumber()]; !dup {
			m[ch.GetNumber()] = struct{}{}
			c = append(c, ch)
		}
	}
	return c
}
