// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"
	"fmt"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"
)

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
	cfg, err := Load(c)
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
