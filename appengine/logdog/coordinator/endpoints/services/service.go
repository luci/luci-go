// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// server is a Cloud Endpoint service supporting privileged support services.
//
// This endpoint is restricted to LogDog support service accounts.
type server struct{}

// New creates a new authenticating ServicesServer instance.
func New() logdog.ServicesServer {
	return &logdog.DecoratedServices{
		Service: &server{},
		Prelude: func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
			// Only service users may access this endpoint.
			if err := coordinator.IsServiceUser(c); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to authenticate user as a service.")
				if !coordinator.IsMembershipError(err) {
					// Not a membership error. Something went wrong on the server's end.
					return nil, grpcutil.Internal
				}
				return nil, grpcutil.PermissionDenied
			}

			// Enter a datastore namespace based on the message type.
			//
			// We use a type switch here because this is a shared decorator.
			if pbm, ok := req.(endpoints.ProjectBoundMessage); ok {
				if err := coordinator.WithProjectNamespaceNoAuth(&c, config.ProjectName(pbm.GetMessageProject())); err != nil {
					return nil, grpcutil.Internal
				}
			}

			return c, nil
		},
	}
}
