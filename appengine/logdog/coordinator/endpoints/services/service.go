// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// Server is a Cloud Endpoint service supporting privileged support services.
//
// This endpoint is restricted to LogDog support service accounts.
type Server struct {
	coordinator.ServiceBase
}

var _ logdog.ServicesServer = (*Server)(nil)

// Auth is endpoint middleware that asserts that the current user is a member of
// the configured group.
func Auth(c context.Context, svc coordinator.Services) error {
	if err := coordinator.IsServiceUser(c, svc); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to authenticate user as a service.")
		if !coordinator.IsMembershipError(err) {
			// Not a membership error. Something went wrong on the server's end.
			return grpcutil.Internal
		}
		return grpcutil.PermissionDenied
	}
	return nil
}
