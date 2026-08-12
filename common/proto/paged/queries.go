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

// Package paged implements a helper for making paginated Datastore queries.
package paged

import (
	"context"
	"reflect"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// Response is an interface implemented by ListResponses which support page
// tokens.
type Response interface {
	proto.Message
	// GetNextPageToken returns a token to use to fetch the next page of results.
	GetNextPageToken() string
}

// Query executes a query to fetch the given page of results, invoking a
// callback function for each key or entity returned by the query. If the page
// isn't the last of the query, the given response will have its next page token
// set appropriately.
//
// A non-positive limit means to fetch all results starting at the given page
// token in a single page. An empty page token means to start at the first page.
//
// The callback must be a function of one argument, the type of which is either
// *datastore.Key (implies keys-only query) or a pointer to a struct to decode
// the returned entity into. The callback should return an error, which if not
// nil halts the query, and if the error is not datastore.Stop, causes this
// function to return an error as well. See datastore.Run for more information.
// No maximum page size is imposed, use datastore.Stop to enforce one.
func Query[V any](ctx context.Context, lim int32, tok string, rsp Response, q *datastore.Query, cb func(V) error) error {
	// Modify the query with the request parameters.
	if tok != "" {
		cur, err := datastore.DecodeCursor(ctx, tok)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid page token %q", tok)
		}
		q = q.Start(cur)
	}
	if lim > 0 {
		// Peek ahead at the next result to determine if the cursor for the given page size
		// is worth returning. The cursor should be omitted if there are no further results.
		q = q.Limit(lim + 1)
	}

	var cur datastore.Cursor
	it := datastore.RunQuery[V](ctx, q)

	// If the query is not limited and the callback never returns datastore.Stop, the query runs
	// until the end so it's not necessary to set the next page token. If the callback does
	// return datastore.Stop, save the cursor but peek at the next result. Only set the next
	// page token if there is a next result.
	// If the query is limited, the limit is set to one more than the specified value in order
	// to peek at the next result by default. Save the cursor at the limit but peek at the next
	// result. Only set the next page token if there is a next result. The callback may return
	// datastore.Stop ahead of the limit. If it does, save the cursor but peek at the next result
	// Only set the next page token if there is a next result.
	i := int32(0)
	for rslt, err := range it.Results {
		if err != nil {
			return errors.Fmt("failed to fetch entities: %w", err)
		}

		i++
		if cur != nil {
			rspStruct := reflect.ValueOf(rsp).Elem()
			rspStruct.FieldByName("NextPageToken").Set(reflect.ValueOf(cur.String()))
			break
		}
		// Invoke the callback. Per t, it returns one argument (the error).
		err := cb(rslt)

		// Save the cursor if the callback wants to stop or the query is limited and
		// this is the last requested result. In either case peek at the next result.
		if err == datastore.Stop || (i == lim && err == nil) {
			var err error
			cur, err = it.Cursor()
			if err != nil {
				return errors.Fmt("failed to fetch cursor: %w", err)
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
