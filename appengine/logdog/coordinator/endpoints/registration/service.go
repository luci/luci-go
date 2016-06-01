// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package registration

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/api/logdog_coordinator/registration/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// server is a service supporting log stream registration.
type server struct{}

// New creates a new authenticating ServicesServer instance.
func New() logdog.RegistrationServer {
	return &logdog.DecoratedRegistration{
		Service: &server{},
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
				if err := coordinator.WithProjectNamespace(&c, project, coordinator.NamespaceAccessWRITE); err != nil {
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
		log.WithError(err).Errorf(c, "User does not have WRITE access to project.")
		return grpcutil.PermissionDenied

	default:
		return grpcutil.Internal
	}
}
