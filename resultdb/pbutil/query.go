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

package pbutil

import (
	"context"

	"github.com/golang/protobuf/proto"
	"golang.org/x/sync/errgroup"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query queries for results continuously, sending individual items to
// dest channel until the paging query is exhausted or the context is canceled.
// A request must be *pb.QueryTestResultRequest, pb.QueryTestExonerationsRequest
// or *pb.QueryArtifactsRequest. Messages sent to dest are *pb.TestResult,
// *pb.TestExoneration or *pb.Artifact respectively.
//
// Does not return the next page token because ctx can be canceled in the middle
// of a page.
//
// If there are multiple requests in reqs, then runs them all concurrently and
// sends all of their results to dest.
// This is useful to query items of different types, e.g. test results and
// test exonerations.
// Does not limit concurrency.
func Query(ctx context.Context, dest chan<- proto.Message, client pb.ResultDBClient, reqs ...proto.Message) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, req := range reqs {
		// Check req type.
		switch req.(type) {
		case *pb.QueryTestResultsRequest:
		case *pb.QueryTestExonerationsRequest:
		case *pb.QueryArtifactsRequest:
		default:
			panic("req must be *QueryTestResultRequest, *QueryTestExonerationRequest or *QueryArtifactsRequest")
		}

		q := &queryResults{
			client: client,
			// Make a copy because we will be modifying it.
			req:  proto.Clone(req),
			dest: dest,
		}
		eg.Go(func() error {
			return q.run(ctx)
		})
	}
	return eg.Wait()
}

// queryResults implements Query for one request.
type queryResults struct {
	client pb.ResultDBClient
	req    proto.Message
	dest   chan<- proto.Message
}

func (q *queryResults) run(ctx context.Context) error {
	// Prepare a channel for responses such that we can make an RPC as soon as we
	// started consuming the response, as opposed to after the response is
	// completely consumed.
	batchC := make(chan []proto.Message)
	errC := make(chan error, 1)
	go func() {
		defer close(batchC)
		errC <- q.queryResponses(ctx, batchC)
	}()

	// Forward items to q.dest.
	for batch := range batchC {
		for _, item := range batch {
			// Note: selecting on errC here would be a race because the batch
			// goroutine might have been already done, but we still did not send all
			// items to the caller.
			select {
			case <-ctx.Done():
				return <-errC
			case q.dest <- item:
			}
		}
	}
	return <-errC
}

// queryResponses pages through query results and sends item batches to batchC.
func (q *queryResults) queryResponses(ctx context.Context, batchC chan<- []proto.Message) error {
	// The initial token is the current token in the base request.
	token := (q.req).(interface{ GetPageToken() string }).GetPageToken()
	for {
		var batch []proto.Message
		var err error
		batch, token, err = q.call(ctx, token)
		if err != nil || len(batch) == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		select {
		case batchC <- batch:
		case <-ctx.Done():
			return ctx.Err()
		}

		if token == "" {
			return nil
		}
	}
}

// call makes a request with a given page token.
func (q *queryResults) call(ctx context.Context, pageToken string) (batch []proto.Message, nextPageToken string, err error) {
	// Make a call and convert the response to generic types.
	switch req := q.req.(type) {
	case *pb.QueryTestResultsRequest:
		req.PageToken = pageToken
		var res *pb.QueryTestResultsResponse
		res, err = q.client.QueryTestResults(ctx, req)
		if res != nil {
			batch = make([]proto.Message, len(res.TestResults))
			for i, r := range res.TestResults {
				batch[i] = r
			}
			nextPageToken = res.NextPageToken
		}

	case *pb.QueryTestExonerationsRequest:
		req.PageToken = pageToken
		var res *pb.QueryTestExonerationsResponse
		res, err = q.client.QueryTestExonerations(ctx, req)
		if res != nil {
			batch = make([]proto.Message, len(res.TestExonerations))
			for i, r := range res.TestExonerations {
				batch[i] = r
			}
			nextPageToken = res.NextPageToken
		}

	case *pb.QueryArtifactsRequest:
		req.PageToken = pageToken
		var res *pb.QueryArtifactsResponse
		res, err = q.client.QueryArtifacts(ctx, req)
		if res != nil {
			batch = make([]proto.Message, len(res.Artifacts))
			for i, r := range res.Artifacts {
				batch[i] = r
			}
			nextPageToken = res.NextPageToken
		}

	default:
		panic("impossible")
	}

	return
}
