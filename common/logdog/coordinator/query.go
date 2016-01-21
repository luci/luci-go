// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

// Trinary is a 3-value query option type.
type Trinary int

const (
	// Both means that the value should not have an effect.
	Both Trinary = iota
	// Yes is a positive effect.
	Yes
	// No is a negative effect.
	No
)

func (t Trinary) queryValue() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	default:
		return ""
	}
}

// Query is a user-facing LogDog Query.
type Query struct {
	// Path is the query parameter.
	//
	// The path expression may substitute a glob ("*") for a specific path
	// component. That is, any stream that matches the remaining structure qualifies
	// regardless of its value in that specific positional field.
	//
	// An unbounded wildcard may appear as a component at the end of both the
	// prefix and name query components. "**" matches all remaining components.
	//
	// If the supplied path query does not contain a path separator ("+"), it will
	// be treated as if the prefix is "**".
	//
	// Examples:
	//   - Empty ("") will return all streams.
	//   - **/+/** will return all streams.
	//   - foo/bar/** will return all streams with the "foo/bar" prefix.
	//   - foo/bar/**/+/baz will return all streams beginning with the "foo/bar"
	//     prefix and named "baz" (e.g., "foo/bar/qux/lol/+/baz")
	//   - foo/bar/+/** will return all streams with a "foo/bar" prefix.
	//   - foo/*/+/baz will return all streams with a two-component prefix whose
	//     first value is "foo" and whose name is "baz".
	//   - foo/bar will return all streams whose name is "foo/bar".
	//   - */* will return all streams with two-component names.
	Path string
	// Tags is the list of tags to require. The value may be empty if key presence
	// is all that is being asserted.
	Tags map[string]string
	// ContentType, if not empty, restricts results to streams with the supplied
	// content type.
	ContentType string

	// Before, if not zero, specifies that only streams registered at or before
	// the supplied time should be returned.
	Before time.Time
	// After, if not zero, specifies that only streams registered at or after
	// the supplied time should be returned.
	After time.Time

	// Terminated, if not Both, selects logs streams that are/aren't terminated.
	Terminated Trinary

	// Archived, if not Both, selects logs streams that are/aren't archived.
	Archived Trinary

	// Purged, if not Both, selects logs streams that are/aren't purged.
	Purged Trinary

	// State, if true, requests that the query results include the log streams'
	// state.
	State bool
}

// QueryStream is a single stream query result.
type QueryStream struct {
	// Path is the stream path for this result.
	Path types.StreamPath

	// DescriptorProto is the binary LogStreamDescriptor protobuf for this stream.
	DescriptorProto []byte
	// State is the log stream's state. It is nil if the query request's State
	// boolean is false.
	State *StreamState

	// descriptor is the cached decoded LogStreamDescriptor protobuf.
	descriptor *logpb.LogStreamDescriptor
}

// Descriptor returns the unmarshalled LogStreamDescriptor protobuf for this
// query response.
func (qs *QueryStream) Descriptor() (*logpb.LogStreamDescriptor, error) {
	if qs.descriptor == nil {
		desc := logpb.LogStreamDescriptor{}
		if err := proto.Unmarshal(qs.DescriptorProto, &desc); err != nil {
			return nil, err
		}
		qs.descriptor = &desc
	}
	return qs.descriptor, nil

}

// QueryCallback is a callback method type that is used in query requests.
//
// If it returns false, additional callbacks and queries will be aborted.
type QueryCallback func(r *QueryStream) bool

// QueryDecodeError is an error returned when processing a query response
// failed.
type QueryDecodeError struct {
	// Path is the path of the stream where decoding failed.
	Path types.StreamPath
	// Err is the underlying decode error.
	Err error
}

func (e *QueryDecodeError) Error() string {
	return fmt.Sprintf("failed to decode [%s]: %v", e.Path, e.Err)
}

func qde(p types.StreamPath, err error) *QueryDecodeError {
	return &QueryDecodeError{
		Path: p,
		Err:  err,
	}
}

// Query executes a query, invoking the supplied callback once for each query
// result.
func (c *Client) Query(ctx context.Context, q *Query, cb QueryCallback) error {
	req := logs.QueryRequest{
		Path:        q.Path,
		ContentType: q.ContentType,
		Tags:        make([]*logs.LogStreamDescriptorTag, 0, len(q.Tags)),
		State:       q.State,
		Proto:       true,
	}
	if !q.Before.IsZero() {
		req.Older = q.Before.Format(time.RFC3339Nano)
	}
	if !q.After.IsZero() {
		req.Newer = q.After.Format(time.RFC3339Nano)
	}

	req.Terminated = q.Terminated.queryValue()
	req.Archived = q.Archived.queryValue()
	req.Purged = q.Purged.queryValue()

	for k, v := range q.Tags {
		req.Tags = append(req.Tags, &logs.LogStreamDescriptorTag{Key: k, Value: v})
	}

	// Iteratively query until either our query is done (Next is empty) or we are
	// asked to stop via callback.
	for {
		resp, err := c.svc.Query(&req).Context(ctx).Do()
		if err != nil {
			return normalizeError(err)
		}

		for _, s := range resp.Streams {
			qresp := QueryStream{
				Path: types.StreamPath(s.Path),
			}
			if s.State != nil {
				qresp.State, err = loadLogStreamState(s.State)
				if err != nil {
					return qde(qresp.Path, err)
				}
			}
			if s.DescriptorProto != "" {
				qresp.DescriptorProto, err = base64.StdEncoding.DecodeString(s.DescriptorProto)
			}
			if !cb(&qresp) {
				return nil
			}
		}

		// Advance our query cursor.
		if resp.Next == "" {
			return nil
		}
		req.Next = resp.Next
	}
}
