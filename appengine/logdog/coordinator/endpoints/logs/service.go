// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// Server is the user-facing log access and query endpoint service.
type server struct {
	// resultLimit is the maximum number of query results to return in a
	// single query. If zero, the default will be used.
	//
	// This is provided for testing purposes.
	resultLimit int
}

// New creates a new authenticating LogsServer instance.
func New() logdog.LogsServer {
	return newService(&server{})
}

func newService(svr *server) logdog.LogsServer {
	return &logdog.DecoratedLogs{
		Service: svr,
		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			// Enter a datastore namespace based on the message type.
			//
			// We use a type switch here because this is a shared decorator. All user
			// mesages must implement ProjectBoundMessage.
			pbm, ok := req.(endpoints.ProjectBoundMessage)
			if ok {
				// Enter the requested project namespace. This validates that the
				// current user has READ access.
				project := config.ProjectName(pbm.GetMessageProject())
				log.Fields{
					"project": project,
				}.Debugf(c, "User is accessing project.")
				if err := coordinator.WithProjectNamespace(&c, project); err != nil {
					return nil, getGRPCError(c, err)
				}
			}

			return c, nil
		},
	}
}

func getGRPCError(c context.Context, err error) error {
	switch {
	case err == nil:
		return nil

	case err == config.ErrNoConfig:
		log.WithError(err).Errorf(c, "No project configuration defined.")
		return grpcutil.PermissionDenied

	case coordinator.IsMembershipError(err):
		log.WithError(err).Errorf(c, "User does not have READ access to project.")
		return grpcutil.PermissionDenied

	default:
		return grpcutil.Internal
	}
}

func (s *server) limit(v int, d int) int {
	if s.resultLimit > 0 {
		d = s.resultLimit
	}
	if v <= 0 || v > d {
		return d
	}
	return v
}
