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

// cursorCBType is the reflect.Type of a datastore.CursorCB.
var cursorCBType = reflect.TypeOf((datastore.CursorCB)(nil))

// returnedNil is a []reflect.Value{} containing one nil error.
var returnedNil = reflect.ValueOf(func() error { return nil }).Call([]reflect.Value{})

// returnedStop is a []reflect.Value{} containing one datastore.Stop error.
var returnedStop = reflect.ValueOf(func() error { return datastore.Stop }).Call([]reflect.Value{})

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
func Query(ctx context.Context, lim int32, tok string, rsp Response, q *datastore.Query, cb any) error {
	// Validate as much about the callback as this function relies on.
	// The rest is validated by datastore.Run.
	v := reflect.ValueOf(cb)
	if v.Kind() != reflect.Func {
		return errors.Reason("callback must be a function").Err()
	}
	t := v.Type()
	switch {
	case t.NumIn() != 1:
		return errors.Reason("callback function must accept one argument").Err()
	case t.NumOut() != 1:
		return errors.Reason("callback function must return one value").Err()
	}

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

	// Wrap the callback with a custom function that grabs the cursor (if necessary) before
	// invoking the callback for each result up to the page size specified in the request.
	// This is the type of function datastore.Run will receive as an argument.
	// TODO(smut): Move this block to gae/datastore, since it doesn't depend on PagedRequest.
	t = reflect.FuncOf([]reflect.Type{t.In(0), cursorCBType}, []reflect.Type{t.Out(0)}, false)
	var cur datastore.Cursor

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
	curCB := reflect.MakeFunc(t, func(args []reflect.Value) []reflect.Value {
		i++
		if cur != nil {
			// Cursor is set below, when the result is at the limit or datastore.Stop
			// is returned by the callback. Since the query is still running, there
			// are more results. Set the page token and halt the query. Don't invoke
			// the callback since it isn't expecting any more results.
			f := reflect.ValueOf(rsp).Elem().FieldByName("NextPageToken")
			f.SetString(cur.String())
			return returnedStop
		}
		// Invoke the callback. Per t, it returns one argument (the error).
		ret := v.Call([]reflect.Value{args[0]})
		// Save the cursor if the callback wants to stop or the query is limited and
		// this is the last requested result. In either case peek at the next result.
		if ret[0].Interface() == datastore.Stop || (i == lim && ret[0].IsNil()) {
			var err error
			cur, err = args[1].Interface().(datastore.CursorCB)()
			if err != nil {
				return []reflect.Value{reflect.ValueOf(errors.Annotate(err, "failed to fetch cursor").Err())}
			}
			return returnedNil
		}
		return ret
	}).Interface()

	if err := datastore.Run(ctx, q, curCB); err != nil {
		return errors.Annotate(err, "failed to fetch entities").Err()
	}
	return nil
}
