// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"golang.org/x/net/context"
)

// ListResult is a single returned list entry.
type ListResult struct {
	// Base is the base part of the list path. This will match the base that the
	// list was pulled from.
	Base string
	// Path is this list component's remaining path, relative to Base.
	Path string
	// Stream is true if this list component is a stream component. Otherwise,
	// it's an intermediate path component.
	Stream bool

	// State is the state of the log stream. This will be populated if this is a
	// stream component and state was requested.
	State *LogStream
}

// FullPath returns the full StreamPath of this result. If it is not a stream,
// it will return an empty path.
func (lr *ListResult) FullPath() types.StreamPath {
	if !lr.Stream {
		return ""
	}
	return types.StreamPath(types.Construct(lr.Base, lr.Path))
}

// ListCallback is a callback method type that is used in list requests.
//
// If it returns false, additional callbacks and list requests will be aborted.
type ListCallback func(*ListResult) bool

// ListOptions is a set of list request options.
type ListOptions struct {
	// Recursive, if true, requests that the list results include the descendents
	// of the components that are immediately under the supplied base.
	Recursive bool
	// StreamsOnly requests that only stream components are returned. Intermediate
	// path components will be omitted.
	StreamsOnly bool
	// State, if true, requests that the full log stream descriptor and state get
	// included in returned stream list components.
	State bool

	// Purged, if true, requests that purged log streams are included in the list
	// results. This will result in an error if the user is not privileged to see
	// purged logs.
	Purged bool
}

// List executes a log stream hierarchy listing for the specified path.
func (c *Client) List(ctx context.Context, base string, o ListOptions, cb ListCallback) error {
	req := logdog.ListRequest{
		Path:          base,
		Recursive:     o.Recursive,
		StreamOnly:    o.StreamsOnly,
		State:         o.State,
		IncludePurged: o.Purged,
	}

	for {
		resp, err := c.C.List(ctx, &req)
		if err != nil {
			return normalizeError(err)
		}

		for _, s := range resp.Components {
			lr := ListResult{
				Base:   resp.Base,
				Path:   s.Path,
				Stream: s.Stream,
			}

			if s.State != nil {
				lr.State = loadLogStream(types.StreamPath(s.Path), s.State, s.Desc)
			}
			if !cb(&lr) {
				return nil
			}
		}

		if resp.Next == "" {
			return nil
		}
		req.Next = resp.Next
	}
}
