// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

// SetConfigRequest is the user-submitted configuration data. It will be loaded
// into a config.GlobalConfig instance.
type SetConfigRequest struct {
	// ConfigServiceURL is the API URL of the base "luci-config" service. If
	// empty, the defualt service URL will be used.
	ConfigServiceURL string `json:"configService" endpoints:"req"`

	// ConfigSet is the name of the configuration set to load from.
	ConfigSet string `json:"configSet" endpoints:"req"`
	// ConfigPath is the path of the text-serialized configuration protobuf.
	ConfigPath string `json:"configPath" endpoints:"req"`
}

// SetConfig loads the supplied configuration into a config.GlobalConfig
// instance.
func (s *Admin) SetConfig(c context.Context, req *SetConfigRequest) error {
	c, err := s.Use(c, MethodInfoMap["SetConfig"])
	if err != nil {
		return err
	}

	// The user must be an administrator.
	if err := config.IsAdminUser(c); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Warningf(c, "User is not an administrator.")

		u := auth.CurrentUser(c)
		if u == nil || !u.Superuser {
			return endpoints.ForbiddenError
		}

		log.Fields{
			"email":    u.Email,
			"clientID": u.ClientID,
			"name":     u.Name,
		}.Infof(c, "User is an AppEngine superuser. Granting access.")
	}

	gcfg := &config.GlobalConfig{
		ConfigServiceURL: req.ConfigServiceURL,
		ConfigSet:        req.ConfigSet,
		ConfigPath:       req.ConfigPath,
	}
	if err := gcfg.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "New configuration did not validate.")
		return endpoints.NewBadRequestError("config did not validate: %v", err)
	}

	if err := gcfg.Store(c, "setConfig endpoint"); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to store new configuration.")
		return endpoints.InternalServerError
	}
	return nil
}
