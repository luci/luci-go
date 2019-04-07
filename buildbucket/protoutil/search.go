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
	"fmt"

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const defaultPageSize = 100

// BuildErr is a tuple (Build, error).
type BuildErr struct {
	*pb.Build
	Err error
}

// Search searches for builds continuously, sending findings to dest
// channel until the search is exhausted or ctx is canceled.
//
// Intermediate BuildErr sent to dest will have non-nil Build and nil Err.
// The last BuildErr sent to dest will have nil Build and might have non-nil
// error.
// The error might be context.Canceled.
// At least one BuildErr will be sent to dest.
//
// It does not return a cursor because it is impossible to return a cursor
// matching the point of the time the context was canceled.
// If the context is not canceled and the search is exhausted, there is no
// cursor to return.
//
// Buffers at most 2*p.BaseReq.PageSize where page size defaults to 100.
func Search(ctx context.Context, dest chan<- BuildErr, client pb.BuildsClient, req *pb.SearchBuildsRequest) {
	// Prepare a channel of responses.
	// This allows sending next request as soon as we started to consume
	// the response, as opposed to after the response is completely consumed.
	type Res struct {
		*pb.SearchBuildsResponse
		err error
	}
	responses := make(chan Res)
	go func() {
		defer close(responses)

		req := proto.Clone(req).(*pb.SearchBuildsRequest)

		// Ensure next_page_token is requested.
		if len(req.GetFields().GetPaths()) > 0 {
			req.Fields.Paths = append(req.Fields.Paths, "next_page_token")
		}

		// Page through results.
		for {
			// Make the RPC.
			res, err := client.SearchBuilds(ctx, req)
			if err != nil {
				responses <- Res{nil, err}
				return
			}

			responses <- Res{res, nil}

			if res.NextPageToken == "" || len(res.Builds) == 0 {
				return
			}

			// Next page...
			req.PageToken = res.NextPageToken
		}
	}()

	// Consume responses and send builds.
	for res := range responses {
		if res.err != nil {
			dest <- BuildErr{nil, res.err}
			return
		}
		for _, b := range res.Builds {
			select {
			case <-ctx.Done():
				// We were interrupted.
				dest <- BuildErr{nil, ctx.Err()}
				return

			case dest <- BuildErr{b, nil}:
			}
		}
	}
	dest <- BuildErr{}
}

type searchStream struct {
	c    chan BuildErr
	head *pb.Build
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

// SearchMulti multiplexes multiple search requests into one channel of builds
// ordered newest-to-oldest.
// Return an error if a response does not include a build id.
func SearchMulti(ctx context.Context, dest chan<- BuildErr, client pb.BuildsClient, requests []*pb.SearchBuildsRequest) {
	// Start N concurrent searches.
	chans := make([]chan BuildErr, len(requests))
	for i, req := range requests {
		chans[i] = make(chan BuildErr)
		go Search(ctx, chans[i], client, req)
	}

	// Collect initial heads and initialize a heap of streams.
	streams := make(streamHeap, 0, len(chans))
	for _, c := range chans {
		be := <-c
		switch {
		case be.Build != nil:
			streams = append(streams, searchStream{c: c, head: be.Build})
		case be.Err != nil:
			dest <- be
			return
		}
	}
	heap.Init(&streams)

	// Start merge-joining.
	lastSeenId := int64(-1)
	for len(streams) > 0 {
		// Pop the build with the smallest id among heads of all remaining
		// streams and send it to dest.
		s := heap.Pop(&streams).(searchStream)

		switch {
		case s.head.Id == 0:
			dest <- BuildErr{nil, fmt.Errorf("a build without an ID in response")}
		case s.head.Id != lastSeenId:
			dest <- BuildErr{s.head, nil}
			lastSeenId = s.head.Id
		}

		// Get the next build from the same stream.
		switch be := <-s.c; {
		case be.Err != nil:
			dest <- be
			return

		case be.Build != nil:
			// This stream still has builds.
			s.head = be.Build
			heap.Push(&streams, s)
		}
	}
	dest <- BuildErr{nil, nil}
}
