// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

// SetConfig loads the supplied configuration into a config.GlobalConfig
// instance.
func (s *Admin) SetConfig(c context.Context, req *config.GlobalConfig) error {
	c, err := s.Use(c, MethodInfoMap["SetConfig"])
	if err != nil {
		return err
	}

	// The user must be an administrator.
	if err := config.IsAdminUser(c); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Warningf(c, "User is not an administrator.")

		// If we're on development server, any user can set the initial config.
		if !info.Get(c).IsDevAppServer() {
			u := auth.CurrentUser(c)
			if !(u != nil && u.Superuser) {
				return endpoints.ForbiddenError
			}

			log.Fields{
				"email":    u.Email,
				"clientID": u.ClientID,
				"name":     u.Name,
			}.Infof(c, "User is an AppEngine superuser. Granting access.")
		}
	}

	if err := req.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "New configuration did not validate.")
		return endpoints.NewBadRequestError("config did not validate: %v", err)
	}

	if err := req.Store(c, "setConfig endpoint"); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to store new configuration.")
		return endpoints.InternalServerError
	}
	return nil
}
