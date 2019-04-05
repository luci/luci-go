// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const defaultPageSize = 100

// SearchParams is argument for Search and SearchCont.
type SearchParams struct {
	Client  pb.BuildsClient
	BaseReq *pb.SearchBuildsRequest
	Limit   int // if positive, stop after this number of builds is found
}

// Search searches for builds and returns them.
// It stops when all builds are found or when context is cancelled.
//
// May return context.Canceled.
func Search(ctx context.Context, p SearchParams) (builds []*pb.Build, nextPageToken string, err error) {
	ch := make(chan *pb.Build)
	go func() {
		defer close(ch)
		nextPageToken, err = SearchCont(ctx, ch, p)
	}()

	for b := range ch {
		builds = append(builds, b)
	}
	return
}

// SearchCont searches for builds continuously, sending findings to builds
// channel until the search is exhausted or ctx is canceled.
// May return context.Canceled.
// Blocks on sending.
func SearchCont(ctx context.Context, builds chan<- *pb.Build, p SearchParams) (nextPageToken string, err error) {
	req := proto.Clone(p.BaseReq).(*pb.SearchBuildsRequest)

	// Define a page size.
	if req.PageSize == 0 {
		req.PageSize = defaultPageSize
	}

	sent := 0
	for {
		// Fetch not more than the limit says.
		if p.Limit > 0 {
			toFetch := int32(p.Limit - sent)
			if req.PageSize > toFetch {
				// This is going to be the last page.
				req.PageSize = toFetch
			}
		}

		// Make the RPC.
		res, err := p.Client.SearchBuilds(ctx, req)
		if err != nil {
			return "", err
		}

		// Consume builds.
		for _, b := range res.Builds {
			select {
			case <-ctx.Done():
				// We were interrupted.
				//
				// Do not lie about the nextPageToken.
				// res.NextPageToken points to the build after the last one
				// in the response, not after the one we sent to the channel.
				return "", ctx.Err()

			case builds <- b:
				sent++
				if p.Limit > 0 && sent > p.Limit {
					// We cannot return a correct nextPageToken in this case.
					return "", fmt.Errorf("server returned more builds than we asked for")
				}
			}
		}

		// Exit if we exhausted the search.
		if len(res.Builds) == 0 || res.NextPageToken == "" {
			return res.NextPageToken, nil
		}

		// Page more...
		req.PageToken = res.NextPageToken
	}
}
