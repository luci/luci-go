// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// LoadStreamRequest is the request structure sent to the service's Load
// endpoint.
type LoadStreamRequest struct {
	// Path is the log stream's path.
	Path string `json:"path,omitempty" endpoints:"req"`
}

// LoadStreamResponse is the response structure from the service's Load
// endpoint.
type LoadStreamResponse struct {
	// Path is the log stream's path, or a hash of the log stream's path.
	Path string `json:"path,omitempty"`

	// State is the request log stream. On error, this will be nil.
	State *lep.LogStreamState `json:"state,omitempty"`
	// Descriptor is the serialized LogStreamDescriptor protobuf for this stream.
	Descriptor []byte `json:"descriptor,omitempty"`
}

// LoadStream implements the "service.loadStream" endpoint.
//
// LoadStream returns stream state for the requested stream.
func (s *Service) LoadStream(c context.Context, req *LoadStreamRequest) (*LoadStreamResponse, error) {
	c, err := s.Use(c, MethodInfoMap["LoadStream"])
	if err != nil {
		return nil, err
	}
	if err := Auth(c); err != nil {
		return nil, err
	}

	c = log.SetField(c, "path", req.Path)
	ls, err := coordinator.NewLogStream(req.Path)
	if err != nil {
		return nil, endpoints.NewBadRequestError("Invalid path (%s): %s", req.Path, err)
	}

	if err := ds.Get(c).Get(ls); err != nil {
		if err == ds.ErrNoSuchEntity {
			log.Debugf(c, "LogStream does not exist.")
			return nil, endpoints.NewNotFoundError("Log stream not found: %s", req.Path)
		}

		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to Get LogStream from datastore.")
		return nil, endpoints.InternalServerError
	}

	resp := LoadStreamResponse{
		Path:       string(ls.Path()),
		State:      lep.LoadLogStreamState(ls),
		Descriptor: ls.Descriptor,
	}

	return &resp, nil
}
