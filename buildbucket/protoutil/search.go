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
	"container/heap"
	"context"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
)

// Search searches for builds continuously, sending findings to `buildC`
// until the search is exhausted or context is canceled.
// The builds are ordered newest-to-oldest and deduplicated.
//
// If len(requests) > 1, the builds represent a union of the search requests.
//
// Search does not return a next page token because ctx can be canceled in the
// middle of a page and because Search supports multiple requests.
func Search(ctx context.Context, buildC chan<- *pb.Build, client grpcpb.BuildsClient, requests ...*pb.SearchBuildsRequest) error {
	// Do not leave sub-goroutunes running.
	var wg sync.WaitGroup
	defer wg.Wait()

	// Ensure sub-goroutines end.
	subCtx, cancelSearches := context.WithCancel(ctx)
	defer cancelSearches()

	// Start len(requests) concurrent searches.
	allStreams := make([]searchStream, len(requests))
	for i, req := range requests {
		s := searchStream{
			buildC: make(chan *pb.Build),
			// Create a buffered channel because we will not necessarily
			// read error from all sub-goroutines.
			errC: make(chan error, 1),
		}
		allStreams[i] = s
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.errC <- searchBuilds(subCtx, s.buildC, client, req)
		}()
	}

	// Collect initial stream heads and initialize a heap of active streams.
	activeStreams := make(streamHeap, 0, len(allStreams))
	for _, s := range allStreams {
		select {
		case s.head = <-s.buildC:
			// This stream has some builds.
			activeStreams = append(activeStreams, s)
		case err := <-s.errC:
			if err != nil {
				return err
			}
		}
	}
	heap.Init(&activeStreams)

	// Merge-sort streams.
	lastSentID := int64(-1)
	for len(activeStreams) > 0 {
		// Pop the build with the smallest id amongst active stream heads.
		s := heap.Pop(&activeStreams).(searchStream)
		// Send it to buildC, unless it is a duplicate.
		if s.head.Id != lastSentID {
			select {
			case buildC <- s.head:
				lastSentID = s.head.Id
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Get the next build from the same stream and record it as a new
		// stream head.
		select {
		case s.head = <-s.buildC:
			// This stream is still active.
			heap.Push(&activeStreams, s)
		case err := <-s.errC:
			// This stream is done.
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// searchBuilds is like Search, but does not implement union of search results.
// Sent builds have ids.
func searchBuilds(ctx context.Context, buildC chan<- *pb.Build, client grpcpb.BuildsClient, req *pb.SearchBuildsRequest) error {
	// Prepare a channel of responses, s.t. we make an RPC as soon as we started
	// consuming the response, as opposed to after the response is completely
	// consumed.
	resC := make(chan *pb.SearchBuildsResponse)
	errC := make(chan error)
	go func() {
		err := searchResponses(ctx, resC, client, req)
		close(resC)
		errC <- err
	}()

	// Forward builds.
	for res := range resC {
		for _, b := range res.Builds {
			select {
			case buildC <- b:
			case <-ctx.Done():
				// Note that even if ctx is done, errC is not guaranteed to have nil
				// because the context might have canceled after the goroutine above
				// exited.
				return ctx.Err()
			}
		}
	}
	return <-errC
}

// searchResponses pages through search results and sends search responses to
// resC.
// Builds in resC have ids.
func searchResponses(ctx context.Context, resC chan<- *pb.SearchBuildsResponse, client grpcpb.BuildsClient, req *pb.SearchBuildsRequest) error {
	req = proto.Clone(req).(*pb.SearchBuildsRequest)

	// Ensure next_page_token and build ID are requested.
	if paths := req.GetFields().GetPaths(); len(paths) > 0 {
		pathSet := stringset.NewFromSlice(paths...)
		pathSet.Add("next_page_token")
		pathSet.Add("builds.*.id")
		req.Fields.Paths = pathSet.ToSortedSlice()
	}

	// Page through results.
	for {
		res, err := client.SearchBuilds(ctx, req)
		if err != nil {
			return err
		}

		select {
		case resC <- res:
		case <-ctx.Done():
			return ctx.Err()
		}

		if res.NextPageToken == "" || len(res.Builds) == 0 {
			return nil
		}

		// Next page...
		req.PageToken = res.NextPageToken
	}
}

type searchStream struct {
	buildC chan *pb.Build
	errC   chan error
	head   *pb.Build
}

type streamHeap []searchStream

// Len implements sort.Interface.
func (h streamHeap) Len() int { return len(h) }

// Less implements sort.Interface.
func (h streamHeap) Less(i, j int) bool {
	return h[i].head.Id < h[j].head.Id
}

// Swap implements sort.Interface.
func (h streamHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Swap implements heap.Interface.
func (h *streamHeap) Push(x any) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(searchStream))
}

// Pop implements heap.Interface.
func (h *streamHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
