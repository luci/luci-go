// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const (
	// listResultLimit is the maximum number of log streams that will be
	// returned in a single query. If the user requests more, it will be
	// automatically truncated to this value.
	listResultLimit = 500
)

// List returns log stream paths rooted under the hierarchy.
func (s *Server) List(c context.Context, req *logdog.ListRequest) (*logdog.ListResponse, error) {
	svc := s.GetServices()
	hr := hierarchy.Request{
		Base:       req.Path,
		Recursive:  req.Recursive,
		StreamOnly: req.StreamOnly,
		Limit:      s.limit(int(req.MaxResults), listResultLimit),
		Next:       req.Next,
		Skip:       int(req.Offset),
	}

	// Non-admin users may not request purged results.
	if req.IncludePurged {
		if err := coordinator.IsAdminUser(c, svc); err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Non-superuser requested to see purged paths. Denying.")
			return nil, grpcutil.Errf(codes.PermissionDenied, "non-admin user cannot request purged log paths")
		}

		hr.IncludePurged = true
	}

	l, err := hierarchy.Get(ds.Get(c), hr)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get hierarchy listing.")
		return nil, grpcutil.InvalidArgument
	}

	resp := logdog.ListResponse{
		Base: l.Base,
		Next: l.Next,
	}
	if len(l.Comp) > 0 {
		resp.Components = make([]*logdog.ListResponse_Component, len(l.Comp))

		for i, c := range l.Comp {
			resp.Components[i] = &logdog.ListResponse_Component{
				Path:   c.Name,
				Stream: (c.Stream != ""),
			}
		}
	}

	// Perform additional stream metadata fetch if state is requested. Collect
	// a list of streams to load.
	//
	// TODO(dnj): A good optimization would be to use a common.meter to do the
	// GetMulti calls in a separate goroutine while the hierarchy elements are
	// being loaded.
	if req.State {
		idxMap := make(map[int]*logdog.ListResponse_Component)
		var streams []*coordinator.LogStream

		for i, c := range l.Comp {
			if c.Stream == "" {
				continue
			}

			idxMap[len(streams)] = resp.Components[i]
			streams = append(streams, coordinator.LogStreamFromPath(c.Stream))
		}

		if len(streams) > 0 {
			if err := ds.Get(c).GetMulti(streams); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"count":      len(streams),
				}.Errorf(c, "Failed to load stream descriptor.")
				return nil, grpcutil.Internal
			}

			for sidx, lrs := range idxMap {
				ls := streams[sidx]
				lrs.State = loadLogStreamState(ls)

				lrs.Desc, err = ls.DescriptorValue()
				if err != nil {
					log.Fields{
						log.ErrorKey: err,
						"path":       ls.Path(),
					}.Errorf(c, "Failed to unmarshal descriptor protobuf.")
					return nil, grpcutil.Internal
				}
			}
		}
	}

	log.Fields{
		"count": len(resp.Components),
	}.Infof(c, "List completed successfully.")
	return &resp, nil
}
