// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// LoadStream loads the log stream state.
func (b *Server) LoadStream(c context.Context, req *logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid path (%s): %s", path, err)
	}
	log.Fields{
		"path": path,
	}.Infof(c, "Loading log stream state.")

	ls := coordinator.LogStreamFromPath(path)
	switch err := ds.Get(c).Get(ls); err {
	case nil:
		// The log stream loaded successfully.
		resp := logdog.LoadStreamResponse{
			State: loadLogStreamState(ls),
		}
		if req.Desc {
			resp.Desc = ls.Descriptor
		}
		return &resp, nil

	case ds.ErrNoSuchEntity:
		return nil, grpcutil.Errf(codes.NotFound, "Log stream was not found.")

	default:
		log.WithError(err).Errorf(c, "Failed to load log stream.")
		return nil, grpcutil.Internal
	}
}
