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

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// search searches for builds continuously, sending findings to builds channel
// until the search is exhausted or ctx is canceled.
//
// If len(requests) > 1, sent builds are ordered newest-to-oldest and
// deduplicated.
//
// It does not return a cursor because it is impossible to return a cursor
// matching the point of the time the context was canceled.
// If the context is not canceled and the search is exhausted, there is no
// cursor to return.
//
// Buffers at most 2*p.BaseReq.PageSize where page size defaults to 100.
func Search(ctx context.Context, builds chan<- *pb.Build, client pb.BuildsClient, requests ...*pb.SearchBuildsRequest) error {
	// Do not leave sub-goroutunes running.
	var wg sync.WaitGroup
	defer wg.Wait()

	subCtx, cancelSearches := context.WithCancel(ctx)
	defer cancelSearches()

	// Start N concurrent searches.
	allStreams := make([]searchStream, len(requests))
	for i, req := range requests {
		s := searchStream{
			buildC: make(chan *pb.Build),
			errC:   make(chan error, 1),
		}
		allStreams[i] = s
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.errC <- searchBuilds(subCtx, s.buildC, client, req)
		}()
	}

	// Collect initial heads and initialize a heap of active streams.
	activeStreams := make(streamHeap, 0, len(allStreams))
	for _, s := range allStreams {
		select {
		case s.head = <-s.buildC:
			activeStreams = append(activeStreams, s)
		case err := <-s.errC:
			if err != nil {
				return err
			}
		}
	}
	heap.Init(&activeStreams)

	// Start merge-joining.
	lastSentID := int64(-1)
	for len(activeStreams) > 0 {
		// Pop the build with the smallest id amongst active stream heads and
		// send it to builds channel.
		s := heap.Pop(&activeStreams).(searchStream)
		if s.head.Id != lastSentID {
			select {
			case builds <- s.head:
				lastSentID = s.head.Id
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Get the next build from the same stream.
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

// searchBuilds searches for builds continuously, sending findings to dest
// channel until the search is exhausted or ctx is canceled.
//
// Intermediate BuildErr sent to dest will have non-nil Build and nil Err.
// The last BuildErr sent to dest will have nil Build and might have non-nil
// error.
// At least one BuildErr will be sent to dest.
//
// It does not return a cursor because it is impossible to return a cursor
// matching the point of the time the context was canceled.
// If the context is not canceled and the search is exhausted, there is no
// cursor to return.
//
// Buffers at most 2*p.BaseReq.PageSize where page size defaults to 100.
func searchBuilds(ctx context.Context, builds chan<- *pb.Build, client pb.BuildsClient, req *pb.SearchBuildsRequest) error {
	// Prepare a channel of responses, s.t. we make an RPC as soon as we started
	//  to consume the response, as opposed to after the response is completely
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
			case <-ctx.Done():
				return <-errC
			case builds <- b:
			}
		}
	}
	return <-errC
}

// searchResponses pages through search results and sends search responses to
// resC.
func searchResponses(ctx context.Context, resC chan<- *pb.SearchBuildsResponse, client pb.BuildsClient, req *pb.SearchBuildsRequest) error {
	req = proto.Clone(req).(*pb.SearchBuildsRequest)

	// Ensure next_page_token and build id are requested.
	if len(req.GetFields().GetPaths()) > 0 {
		req.Fields.Paths = append(req.Fields.Paths, "next_page_token", "builds.*.id")
	}

	// Page through results.
	for {
		// Make an RPC.
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
func (h *streamHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(searchStream))
}

// Pop implements heap.Interface.
func (h *streamHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
