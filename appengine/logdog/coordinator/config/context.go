// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	gaeauthClient "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/remote"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"
)

// Install loads/caches the global application Coordinator configuration
// settings.
//
// It also installs the "luci-config" configuration into the Context. If no
// "luci-config" configuration is defined, it will be ignored.
func Install(c context.Context) (context.Context, error) {
	// Load the global configuration.
	gc, err := Load(c)
	if err != nil {
		return nil, err
	}

	// Prepare the "luci-config" parameters.
	//
	// Use an e-mail OAuth2-authenticated transport to pull from "luci-config".
	c = gaeauthClient.UseServiceAccountTransport(c, nil, nil)
	c = remote.Use(c, gc.ConfigServiceURL)

	// Add a memcache-based caching filter.
	c = gaeconfig.AddFilter(c, gaeconfig.DefaultExpire)

	return c, nil
}

// Get loads the "luci-config" protobuf from the current Context.
func Get(c context.Context) (*Config, error) {
	// Load the global configuration.
	gc, err := Load(c)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Get(c).GetConfig(gc.ConfigSet, gc.ConfigPath, false)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"serviceURL": gc.ConfigServiceURL,
			"configSet":  gc.ConfigSet,
			"configPath": gc.ConfigPath,
		}.Errorf(c, "Failed to load config.")
		return nil, errors.New("unable to load config")
	}

	cc := Config{}
	if err := proto.UnmarshalText(cfg.Content, &cc); err != nil {
		log.Fields{
			log.ErrorKey:  err,
			"size":        len(cfg.Content),
			"contentHash": cfg.ContentHash,
			"configSet":   cfg.ConfigSet,
			"revision":    cfg.Revision,
		}.Errorf(c, "Failed to unmarshal configuration protobuf.")
		return nil, errors.New("configuration is invalid or corrupt")
	}

	return &cc, nil
}

// IsAdminUser tests whether the current user belongs to the administrative
// users group. It will return an error if the user does not.
func IsAdminUser(c context.Context) error {
	return isMember(c, func(cfg *Config) string {
		return cfg.AdminAuthGroup
	})
}

// IsServiceUser tests whether the current user belongs to the backend services
// users group. It will return an error if the user does not.
func IsServiceUser(c context.Context) error {
	return isMember(c, func(cfg *Config) string {
		return cfg.ServiceAuthGroup
	})
}

func isMember(c context.Context, groupNameFunc func(*Config) string) error {
	cfg, err := Get(c)
	if err != nil {
		return err
	}

	groupName := groupNameFunc(cfg)
	if groupName == "" {
		return errors.New("no auth group is configured")
	}

	is, err := auth.IsMember(c, groupName)
	if err != nil {
		return err
	}
	if !is {
		return &MembershipError{
			Identity: auth.CurrentIdentity(c),
			Group:    groupName,
		}
	}
	return nil
}

// MembershipError is an error returned by group membership checking functions
// if the current identity is not a member of the requested group.
type MembershipError struct {
	Identity identity.Identity
	Group    string
}

func (e *MembershipError) Error() string {
	return fmt.Sprintf("user %q is not a member of group %q", e.Identity, e.Group)
}

// IsMembershipError returns whether a given error is a membership error.
func IsMembershipError(e error) bool {
	_, ok := e.(*MembershipError)
	return ok
}
