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

package proto

import (
	"context"
	"reflect"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
)

// PagedRequest is an interface implemented by ListRequests which support
// page tokens and page sizes.
type PagedRequest interface {
	proto.Message
	// GetPageSize returns the maximum number of results to fetch.
	GetPageSize() int32
	// GetPageToken returns a token for fetching a specific page of results.
	GetPageToken() string
}

// defPageSize is the default value for the maximum number of results to return
// for a given PagedRequest.
const defPageSize = 200

// PagedResponse is an interface implemented by ListResponses which support
// page tokens.
type PagedResponse interface {
	proto.Message
	// GetNextPageToken returns a token to use to fetch the next page of results.
	GetNextPageToken() string
}

// cursorCBType is the reflect.Type of a datastore.CursorCB.
var cursorCBType = reflect.TypeOf((datastore.CursorCB)(nil))

// returnedNil is a []reflect.Value{} containing one nil error.
var returnedNil = reflect.ValueOf(func() error { return nil }).Call([]reflect.Value{})

// PageQuery executes a query to fetch the results page specified by the given
// request, invoking a callback function for each key or entity returned by the
// query. If the page isn't the last of the query, the given response will have
// its next page token set appropriately.
//
// The callback must be a function of one argument, the type of which is either
// *datastore.Key (implies keys-only query) or a pointer to a struct to decode
// the returned entity into. The callback should return an error, which if not
// nil halts the query, and if the error is not datastore.Stop, causes this
// function to return an error as well. See datastore.Run for more information.
// No maximum page size is imposed, use datastore.Stop to enforce one.
func PageQuery(c context.Context, req PagedRequest, rsp PagedResponse, q *datastore.Query, cb interface{}) error {
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
	if tok := req.GetPageToken(); tok != "" {
		cur, err := datastore.DecodeCursor(c, tok)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid page token %q", tok)
		}
		q = q.Start(cur)
	}
	lim := req.GetPageSize()
	if lim < 1 {
		lim = defPageSize
	}
	// Peek ahead at the next result to determine if the cursor for the given page size
	// is worth returning. The cursor should be omitted if there are no further results.
	q = q.Limit(lim + 1)

	// Wrap the callback with a custom function that grabs the cursor (if necessary) before
	// invoking the callback for each result up to the page size specified in the request.
	// This is the type of function datastore.Run will receive as an argument.
	// TODO(smut): Move this block to gae/datastore, since it doesn't depend on PagedRequest.
	t = reflect.FuncOf([]reflect.Type{t.In(0), cursorCBType}, []reflect.Type{t.Out(0)}, false)
	var cur datastore.Cursor
	i := int32(0)
	curCB := reflect.MakeFunc(t, func(args []reflect.Value) []reflect.Value {
		i++
		// If this result is past the specified page size, don't invoke the callback.
		if i > lim {
			return returnedNil
		}
		// Invoke the callback. Per t, it returns one argument (the error).
		ret := v.Call([]reflect.Value{args[0]})
		// Fetch the cursor if this is the last requested record.
		if (i == lim && ret[0].IsNil()) || ret[0].Interface() == datastore.Stop {
			var err error
			cur, err = args[1].Interface().(datastore.CursorCB)()
			if err != nil {
				return []reflect.Value{reflect.ValueOf(errors.Annotate(err, "failed to fetch cursor").Err())}
			}
			// Fetch the next result, if there is one, without invoking the callback
			// by overriding the return value to not halt the query and setting i to
			// the last requested result.
			i = lim
			ret = returnedNil
		}
		return ret
	}).Interface()

	if err := datastore.Run(c, q, curCB); err != nil {
		return errors.Annotate(err, "failed to fetch entities").Err()
	}

	// rsp.NextPageToken should be set if q had more results than the requested
	// page size or after datastore.Stop was returned by the callback. In any other
	// case, there are no more results so the cursor should be omitted.
	if i > lim && cur != nil {
		v := reflect.ValueOf(rsp).Elem()
		f := v.FieldByName("NextPageToken")
		f.SetString(cur.String())
	}
	return nil
}
