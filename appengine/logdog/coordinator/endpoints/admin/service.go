// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

// Server is the Cloud Endpoint service structure for the administrator endpoint.
type Server struct{}

var _ logdog.AdminServer = (*Server)(nil)

// Auth returns an error if the current user does not have access to
// adminstrative endpoints.
func (s *Server) Auth(c context.Context) error {
	if err := coordinator.IsAdminUser(c); err != nil {
		log.WithError(err).Warningf(c, "User is not an administrator.")

		// If we're on development server, any user can access this endpoint.
		if info.Get(c).IsDevAppServer() {
			log.Infof(c, "On development server, allowing admin access.")
			return nil
		}

		u := auth.CurrentUser(c)
		if !(u != nil && u.Superuser) {
			return grpcutil.PermissionDenied
		}

		log.Fields{
			"email":    u.Email,
			"clientID": u.ClientID,
			"name":     u.Name,
		}.Infof(c, "User is an AppEngine superuser. Granting access.")
	}

	return nil
}
