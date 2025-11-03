// Copyright 2018 The LUCI Authors.
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

package gitiles

import (
	"context"
	"iter"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
)

// DefaultLimit is the default maximum number of commits to load in PagingLog.
const DefaultLimit = 1000

// NoLimit is a value IterLog limit which indicates to iterate until the log is
// exhausted.
const NoLimit = -1

// Helper functions for Gitiles.Log RPC.

// PagingLog is a wrapper around Gitiles.Log RPC that pages though commits.
// If req.PageToken is not empty, paging will continue from there.
//
// req.PageSize specifies maximum number of commits to load in each page.
//
// Limit specifies the maximum number of commits to load.
// 0 means use DefaultLimit.
//
// Deprecated: Prefer IterLog instead; This just buffers the result of IterLog
// into a slice. If you can process commits one at a time, IterLog will allow
// you to make fewer RPCs if your processing can end early.
func PagingLog(ctx context.Context, client pagingLogClient, req *gitiles.LogRequest, limit int, opts ...grpc.CallOption) ([]*git.Commit, error) {
	switch {
	case limit < 0:
		return nil, errors.New("gitiles.PagingLog: limit must not be negative")
	case limit == 0:
		limit = DefaultLimit
	}

	combinedLog := make([]*git.Commit, 0, limit)

	for commit, err := range IterLog(ctx, client, req, limit, opts...) {
		if err != nil {
			return combinedLog, err
		}
		combinedLog = append(combinedLog, commit)
		if len(combinedLog) == limit {
			break
		}
	}

	return combinedLog, nil
}

type pagingLogClient interface {
	Log(context.Context, *gitiles.LogRequest, ...grpc.CallOption) (*gitiles.LogResponse, error)
}

// IterLog returns an iterator which yields commits, or an error if one of the
// pages failed to load.
//
// If the iterator returns an error, it also terminates the iteration.
//
// Internally, this will use `req.PageSize` for the size of a page chunk to
// load.
//
// If req.PageToken is set, this will start iterating from that page.
//
// `limit` specifies an absolute upper limit of commits to yield; This will be
// used to adjust `req.PageSize` for internal page queries.
// If it's NoLimit, then no limit applies.
// If it's 0, defaults to NoLimit.
//
// Usage:
//
//	  for commit, err := range gitiles.IterLog(ctx, client, req, gitiles.NoLimit) {
//		   if err != nil {
//		     return err
//		   }
//		   // handle commit
//	  }
func IterLog(ctx context.Context, client pagingLogClient, req *gitiles.LogRequest, limit int, opts ...grpc.CallOption) iter.Seq2[*git.Commit, error] {
	// req needs to mutate, so clone it.
	req = proto.Clone(req).(*gitiles.LogRequest)

	if limit == 0 {
		limit = NoLimit
	}

	return func(yield func(*git.Commit, error) bool) {
		if limit < NoLimit {
			yield(nil, errors.Fmt("gitiles.IterLog used with invalid limit: %d", limit))
			return
		}

		yielded := 0

		for {
			if limit != NoLimit {
				if remaining := limit - yielded; req.PageSize == 0 || req.PageSize > int32(remaining) {
					req.PageSize = int32(remaining)
				}
			}

			rsp, err := client.Log(ctx, req, opts...)
			if err != nil {
				yield(nil, err)
				return
			}
			for _, commit := range rsp.Log {
				if !yield(commit, nil) {
					return
				}
				yielded += 1
				if limit != NoLimit && yielded > limit {
					return
				}
			}
			if rsp.NextPageToken == "" {
				return
			}
			req.PageToken = rsp.NextPageToken
		}
	}
}
