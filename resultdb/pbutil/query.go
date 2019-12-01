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

package pbutil

import (
	"context"

	"github.com/golang/protobuf/proto"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// QueryResults queries for results continuously, sending individual items to
// itemC until the paging query is exhausted or the context is canceled.
// req must be *pb.QueryTestResultRequest or *pb.QueryTestExonerationsRequest.
// Messages sent to itemC are *pb.TestResult or *pb.TestExoneration
// respectively.
//
// Does not return a next page token because ctx can be canceled in the middle
// of a page.
//
// The order of items is undefined.
func QueryResults(ctx context.Context, itemC chan<- proto.Message, client pb.ResultDBClient, req proto.Message) error {
	// Check req type.
	switch req.(type) {
	case *pb.QueryTestResultsRequest:
	case *pb.QueryTestExonerationsRequest:
	default:
		panic("req is neither QueryTestResultRequest nor QueryTestExonerationRequest")
	}

	q := &queryResults{
		client:  client,
		baseReq: req,
		itemC:   itemC,
	}
	return q.run(ctx)
}

// queryResults implements QueryResults.
type queryResults struct {
	client  pb.ResultDBClient
	baseReq proto.Message
	itemC   chan<- proto.Message
}

func (q *queryResults) run(ctx context.Context) error {
	// Prepare a channel of responses, such that we make an RPC as soon as we
	// started consuming the response, as opposed to after the response is
	// completely consumed.
	batchC := make(chan []proto.Message)
	errC := make(chan error)
	go func() {
		err := q.queryResponses(ctx, batchC)
		close(batchC)
		errC <- err
	}()

	// Forward items to q.itemC.
	for batch := range batchC {
		for _, item := range batch {
			select {
			case q.itemC <- item:

			case <-ctx.Done():
				// Note: selecting on errC here would be a race because the batch
				// goroutine might have been already done, but we still did not send all
				// items to the caller.
				return <-errC
			}
		}
	}
	return <-errC
}

// queryResponses pages through query results and sends batches of item batches
// to batchC.
func (q *queryResults) queryResponses(ctx context.Context, batchC chan<- []proto.Message) error {
	// Page through results.
	// The first token is the current token in the base request.
	token := (q.baseReq).(interface{ GetPageToken() string }).GetPageToken()
	for {
		var batch []proto.Message
		var err error
		if batch, token, err = q.call(ctx, token); err != nil {
			return err
		}

		select {
		case batchC <- batch:
		case <-ctx.Done():
			return ctx.Err()
		}

		if token == "" || len(batch) == 0 {
			return nil
		}
	}
}

// call makes a request with a given page token.
func (q *queryResults) call(ctx context.Context, pageToken string) (batch []proto.Message, nextPageToken string, err error) {
	// Make a shallow copy of the request and assign the page token.
	// Make a call and convert the response to generic types.
	switch req := q.baseReq.(type) {
	case *pb.QueryTestResultsRequest:
		cpy := *req
		cpy.PageToken = pageToken
		var res *pb.QueryTestResultsResponse
		res, err = q.client.QueryTestResults(ctx, &cpy)
		if res != nil {
			batch = make([]proto.Message, len(res.TestResults))
			for i, r := range res.TestResults {
				batch[i] = r
			}
			nextPageToken = res.NextPageToken
		}

	case *pb.QueryTestExonerationsRequest:
		cpy := *req
		cpy.PageToken = pageToken
		var res *pb.QueryTestExonerationsResponse
		res, err = q.client.QueryTestExonerations(ctx, &cpy)
		if res != nil {
			batch = make([]proto.Message, len(res.TestExonerations))
			for i, r := range res.TestExonerations {
				batch[i] = r
			}
			nextPageToken = res.NextPageToken
		}

	default:
		panic("impossible")
	}

	return
}
