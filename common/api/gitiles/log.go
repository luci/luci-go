package gitiles

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
)

const DefaultLimit = 1000

// Helper functions for Gitiles.Log RPC.

// PagingLog is a wrapper around Gitiles.Log RPC that pages till all commits are loaded.
// If req.PageToken is not empty, paging will continue from there.
//
// Limit specifies the maximum number of commits to load.
// 0 means use DefaultLimit.
func PagingLog(ctx context.Context, client gitiles.GitilesClient, req gitiles.LogRequest, limit int, opts ...grpc.CallOption) ([]*git.Commit, error) {
	// Note: we intentionally receive req as struct (not pointer)
	// because we need to mutate it.

	switch {
	case limit < 0:
		return nil, errors.New("limit must not be negative")
	case limit == 0:
		limit = DefaultLimit
	}

	var combinedLog []*git.Commit
	for {
		if remaining := limit - len(combinedLog); remaining <= 0 {
			break
		} else if req.PageSize == 0 || remaining < int(req.PageSize) {
			// Do not fetch more than we need.
			req.PageSize = int32(remaining)
		}

		res, err := client.Log(ctx, &req, opts...)
		if err != nil {
			return combinedLog, err
		}

		// req was capped, so this should not exceed limit.
		combinedLog = append(combinedLog, res.Log...)
		req.PageToken = res.NextPageToken
	}
	return combinedLog, nil
}

// LogForward is a hacky wrapper over Gitiles.Log RPC to get a list of commits
// that goes forward instead of backwards.
//
// XXXXXXXXX
// The response for Log will always include rx and its immediate
// ancestors (rx-1, rx-2, ..., rx-n) but may not reach r1 necessarily  if the
// delta between the commits reachable from rx and those reachable from r0 is
// larger than the a given limit; LogForward will keep paging back until it
// reaches r1 and will return the last page or two in reverse order (r1, r2, r3,
// r4, ..., r~PageSize).
//
// req.PageSize specifies the approximate size of the
// page.
//
// Note that this function is designed to return at least
// req.PageSize commits if they are available, and not to exceed twice
// that amount. This variability is caused because there is no way to know in
// advance how big the last page will be, and there is no significant memory
// savings in truncating the result, thus any truncation is left to the caller.
//
// WARNING: This API pages back from rx until r0 (or any commit reachable
// from r0), and it makes an Gitiles.Log RPC for each page. It is strongly
// recommended to use large page size, or else this API will generate an
// *excessive* number of requests.
//
// WARNING: Beware of paging using sequence of LogForward calls. Not only is it
// inefficient, but it may also miss commits if range r0..rx contains merge
// commits (with 2+ parents).
//
// In pseudocode:
//   if
//     all := reverse(Log(r0, rX))
//     oldest := LogForward(r0, rX)
//     newest := LogForward(oldest[len(oldest)-1].commit, rX)
//   then
//     all **may have more commits than** (oldest + newest)
func LogForward(ctx context.Context, client gitiles.GitilesClient, req gitiles.LogRequest, opts ...grpc.CallOption) ([]*git.Commit, error) {
	// Note: we intentionally receive req as struct (not pointer)
	// because we need to mutate it.

	var pp []*git.Commit
loop:
	for {
		res, err := client.Log(ctx, &req, opts...)
		switch {
		case err != nil:
			return nil, err
		case res.NextPageToken == "":
			pp = append(pp, res.Log...)
			break loop
		default:
			// Keep the previous page.
			pp = res.Log
			req.PageToken = res.NextPageToken
		}
	}

	for i, j := 0, len(pp)-1; i < j; i, j = i+1, j-1 {
		pp[i], pp[j] = pp[j], pp[i]
	}
	return pp, nil
}
