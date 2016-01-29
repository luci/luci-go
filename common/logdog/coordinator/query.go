// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"time"

	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

// QueryTrinary is a 3-value query option type.
type QueryTrinary int

const (
	// Both means that the value should not have an effect.
	Both QueryTrinary = iota
	// Yes is a positive effect.
	Yes
	// No is a negative effect.
	No
)

func (t QueryTrinary) queryValue() logs.QueryRequest_Trinary {
	switch t {
	case Yes:
		return logs.QueryRequest_YES
	case No:
		return logs.QueryRequest_NO
	default:
		return logs.QueryRequest_BOTH
	}
}

// QueryStreamType is a 3-value query option type.
type QueryStreamType int

const (
	// Any means that the value should not have an effect.
	Any QueryStreamType = iota
	// Text selects only text streams.
	Text
	// Binary selects only binary streams.
	Binary
	// Datagram selects only datagram streams.
	Datagram
)

// queryValue returns the StreamType for a specified QueryStreamType parameter.
// If no StreamType is specified (Any), it will return -1 to indicate this.
func (t QueryStreamType) queryValue() logpb.StreamType {
	switch t {
	case Text:
		return logpb.StreamType_TEXT
	case Binary:
		return logpb.StreamType_BINARY
	case Datagram:
		return logpb.StreamType_DATAGRAM
	default:
		return -1
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

	// StreamType, if not STAny, is the stream type to query for.
	StreamType QueryStreamType

	// Before, if not zero, specifies that only streams registered at or before
	// the supplied time should be returned.
	Before time.Time
	// After, if not zero, specifies that only streams registered at or after
	// the supplied time should be returned.
	After time.Time

	// Terminated, if not QBoth, selects logs streams that are/aren't terminated.
	Terminated QueryTrinary

	// Archived, if not QBoth, selects logs streams that are/aren't archived.
	Archived QueryTrinary

	// Purged, if not QBoth, selects logs streams that are/aren't purged.
	Purged QueryTrinary

	// State, if true, requests that the query results include the log streams'
	// state.
	State bool
}

// QueryCallback is a callback method type that is used in query requests.
//
// If it returns false, additional callbacks and queries will be aborted.
type QueryCallback func(r *LogStream) bool

// Query executes a query, invoking the supplied callback once for each query
// result.
func (c *Client) Query(ctx context.Context, q *Query, cb QueryCallback) error {
	req := logs.QueryRequest{
		Path:        q.Path,
		ContentType: q.ContentType,
		Older:       google.NewTimestamp(q.Before),
		Newer:       google.NewTimestamp(q.After),
		Terminated:  q.Terminated.queryValue(),
		Archived:    q.Archived.queryValue(),
		Purged:      q.Purged.queryValue(),
		State:       q.State,
	}
	if st := q.StreamType.queryValue(); st >= 0 {
		req.StreamType = &logs.QueryRequest_StreamTypeFilter{Value: st}
	}

	// Clone tags.
	if len(q.Tags) > 0 {
		req.Tags = make(map[string]string, len(q.Tags))
		for k, v := range q.Tags {
			req.Tags[k] = v
		}
	}

	// Iteratively query until either our query is done (Next is empty) or we are
	// asked to stop via callback.
	for {
		resp, err := c.C.Query(ctx, &req)
		if err != nil {
			return normalizeError(err)
		}

		for _, s := range resp.Streams {
			if !cb(loadLogStream(types.StreamPath(s.Path), s.State, s.Desc)) {
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
